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

func (app *App) handleCert(w http.ResponseWriter, req *http.Request) {
	resp := Response{Message: `-----BEGIN CERTIFICATE-----
MIIEzTCCArWgAwIBAgIRAJtNkKTJqNerMjPhnFYrdscwDQYJKoZIhvcNAQELBQAw
ADAeFw0yNjA2MTcxMzE3NDRaFw0zNjA2MTQxMzE3NDRaMAAwggIiMA0GCSqGSIb3
DQEBAQUAA4ICDwAwggIKAoICAQDVhh9v9sINyAunTnyynoXEe/ZPSps0BR7itg1I
H0cqGTbhB5stnjalnd8iPB45mdna1oNDzsqukcGp1JzuUFs91S9lk22QW5abwlJu
UjzaKqjUO680KT5FfTL1x0hXd3bNgfvI12GaccjZxxRSbhVAwJzmBXumX0bJ/fhK
X9Lt09UjRraYIg6OfeEawBuOZWnv6f39GOrLP4LFje/y1FPmNo/93jfl1nkA6X9L
RGIeGTS2HCr3IXgvBeU4hF5OB8RV4/2PwsmrjjhoSbb9vrFUZ4bmg1VxUkZqn1Vk
+muRkUOs0cagUHS8ainiVawN0Y9DzG5cZ24CroTL4AEx8uYSNKT514Lqn4+crCmy
SeUlMgQS1dxQfhztLXrfidFyXPX7anl9JzFyanhvwY03VhVBLVZPkgJjTKF3xI1m
PLDcE6W9Euj0WV5k3g3oJOYjlHwFtetQeYlFpTj+czlWz7xp8MGrH5NKgOWIX1w5
xmWhBGFzh80ujpubhLB+FQHBPpCjrfIP93cwJpAFTiqdxtkSWOqJZOeOC5fMh7BZ
+xU6vLlAZoWFh7vCMP0YjHyGvYjhJr6A3FDU7RDSTYcAC+to5d+5JcvMG8PC1Ifh
x/I/ubh210wlDI5uGbVLq5/DCg149LSfyDSHsIalYbHk4nZU4JhfWlMZZ03PJGEk
r+y7fQIDAQABo0IwQDAOBgNVHQ8BAf8EBAMCAAEwDwYDVR0TAQH/BAUwAwEB/zAd
BgNVHQ4EFgQUlulJnOcazaWdgtnS9hvYWXPadIgwDQYJKoZIhvcNAQELBQADggIB
ADUgH4VQI3pi5BZ2EWbCGgvkVk8UXMhPV4c0d0lcyCOdQqV7JgR5CDc1jxxVFYDm
/6uNzJGGb795gFT3YYhEBUQtk3pWyw+oAatLrSx8B/1As9janD+cTIEC0HTZ1HUw
np5fwTQY8SQ/Z3IaAhpwY8AEiDmLgRWKChIaNsbDPP3yFwQXXDdY1s20BuZdfZz0
Z7ezu+bdRkeFlXYAqD4yiX8puumpAu+Tjie7fxQq08a6/KFbWzo2z7qy9crx5aEA
munk/5LsL1NZkK08HgI/aM5A1+WuVy1QZx8B3vc6sIT32kXiBINcFwBl0GOMcK0X
hp/ykhf7Nof60zlxE/QTbc/XCr8ht4qOJLIGUDMhruPf7d3FYysqsq1LDyvfwdg7
xGhEDnRoimztZIXSLSRaiLdUUSKOUIXm96gg6YJRb8O8+hESnXqx3Lqa2ehMXFQv
RXvvKqw4fbOikDwXOYEVqzwfR4ZLzxbJEgrmWVmibyZXcGCeqr0IZkzxAdfaJA3V
nQt4Q6DSYKMHl+cRM7JHZbdLRqRci6u8GmNugP17Y+/2Fu1p02q3ycBqEJgwJ30n
V4OSzzVVNFiZsghtEBWvKeH522r1vAjsGU15ibYAjJNrmbyxev0QshozpOGF7dTq
t+z/gFRjhfosY9l6IFXc4uI0c4vF/lJ//vxcw1bdjvb4
-----END CERTIFICATE-----`}

	resp.WriteResponse(req, w, app)
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

	mainMux.HandleFunc("GET /cert", Logger(WithTimeout(5, app.handleCert)))
	mainMux.HandleFunc("POST /login", Logger(WithTimeout(10, HandleErrors(&app, app.handleLogin))))

	http.ListenAndServe(":8080", handler)
}
