package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/b-zago/rikami-api/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type UserLogin struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type RTokenCacheData struct {
	Permissions Permissions `json:"permissions"`
	HMAC        string      `json:"hmac"`
	UserID      int         `json:"user_id"`
}

type Response struct {
	Message string `json:"message"`
}

type App struct {
	Envs          *Envs
	DefaultErrors *DefaultErrors
	DbPool        *pgxpool.Pool
	RedisClient   *redis.Client
}

type DefaultErrors struct {
	UnexpectedError   json.RawMessage `json:"unexpected"`
	Forbidden         json.RawMessage `json:"forbidden"`
	InsufficientPerms json.RawMessage `json:"insufficientPerms"`
	Timedout          json.RawMessage `json:"timedout"`
	FailedAuth        json.RawMessage `json:"failedAuth"`
	BadRequest        json.RawMessage `json:"badRequest"`
}

var (
	ErrLog  = log.New(os.Stderr, "ERROR: ", log.LstdFlags|log.Lshortfile)
	InfoLog = log.New(os.Stdout, "INFO: ", log.LstdFlags)
)

func (app *App) handleCert(w http.ResponseWriter, req *http.Request) *ServerError {
	cert, err := fetchCert(req.Context(), app.Envs.IN_CLUSTER_CERT_URL)
	if err != nil {
		return NewServerError("failed to fetch cert", http.StatusInternalServerError, err)
	}
	defer cert.Close()
	readCert, err := io.ReadAll(cert)
	if err != nil {
		return NewServerError("failed to fetch cert", http.StatusInternalServerError, err)
	}

	resp := Response{Message: string(readCert)}
	resp.WriteResponse(req, w, app)
	return nil
}

func (app *App) handleLogin(w http.ResponseWriter, req *http.Request) *ServerError {
	body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return NewServerError(fmt.Sprintf("login failed with: %v", err), http.StatusInternalServerError, err)
	}

	var userData UserLogin
	err = json.Unmarshal(body, &userData)
	if err != nil {
		return NewServerError(fmt.Sprintf("reading body failed when tried to login: %v", err), http.StatusInternalServerError, err)
	}

	if userData.Password == "" || userData.User == "" {
		return NewServerError("failed login attempt with bad user data", http.StatusBadRequest, nil)
	}

	sig := req.Header.Get("x-rikami-signature")
	if sig == "" {
		return NewServerError("failed login attempt with no signature provided", http.StatusForbidden, nil)
	}

	user, err := app.getUserByName(req.Context(), userData.User)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return NewServerError(err.Error(), http.StatusForbidden, err)
		}
		return NewServerError(err.Error(), http.StatusInternalServerError, err)
	}

	if !auth.VerifyHMAC([]byte(sig), []byte(user.HMACToken), body) {
		return NewServerError("failed login attempt with bad signature", http.StatusForbidden, nil)
	}

	if !auth.VerifyPassword(userData.Password, user.Pass) {
		return NewServerError("failed login attempt with bad password", http.StatusForbidden, nil)
	}

	InfoLog.Printf("starting generating new set of tokens for user: %s\n", user.UserName)
	tokens, err := auth.NewTokens()
	if err != nil {
		return NewServerError(fmt.Sprintf("login error on generating tokens: %v", err), http.StatusInternalServerError, err)
	}

	tokenData, err := json.Marshal(tokens)
	if err != nil {
		return NewServerError(fmt.Sprintf("login error on parsing genereted tokens: %v", err), http.StatusInternalServerError, err)
	}

	rTokenData := RTokenCacheData{Permissions: user.Perms, HMAC: user.HMACToken, UserID: user.UserID}
	rTokenParsed, err := json.Marshal(&rTokenData)
	if err != nil {
		return NewServerError(fmt.Sprintf("login error on parsing genereted refresh token data: %v", err), http.StatusInternalServerError, err)
	}

	err = app.loginUserTokens(req.Context(), user.UserID, tokenData)
	if err != nil {
		return NewServerError(fmt.Sprintf("login error on redis loginUserTokens: %v", err), http.StatusInternalServerError, err)
	}

	err = app.setTokens(req.Context(), tokens, rTokenParsed)
	if err != nil {
		return NewServerError(fmt.Sprintf("login error on setting tokens: %v", err), http.StatusInternalServerError, err)
	}

	w.Write(tokenData)
	InfoLog.Printf("success login for user: %s\n", user.UserName)
	return nil
}

func main() {
	app := App{}
	app.DefaultErrors = LoadDefaultErrors()
	app.Envs = GetEnvs()

	// 10 seconds for succesful setup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// postgresql connection
	app.NewDbPool(ctx)
	// redis connection
	app.NewRedisClient(ctx)
	// register default admin if no admin accounts exist
	app.RegisterAdmin(ctx)

	mainMux := http.NewServeMux()
	handler := http.TimeoutHandler(mainMux, 5*time.Second, string(app.DefaultErrors.Timedout))
	// main

	mainMux.HandleFunc("GET /cert", Logger(WithTimeout(5, HandleErrors(&app, app.handleCert))))
	mainMux.HandleFunc("POST /login", Logger(WithTimeout(10, HandleErrors(&app, app.handleLogin))))

	http.ListenAndServe(":8080", handler)
}
