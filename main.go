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
	resp := Response{Message: "-----BEGIN CERTIFICATE-----\nMIIEzTCCArWgAwIBAgIRAJTS7990LG/nShvMbY75wxowDQYJKoZIhvcNAQELBQAw\nADAeFw0yNjA1MTgxMzE3NDJaFw0zNjA1MTUxMzE3NDJaMAAwggIiMA0GCSqGSIb3\nDQEBAQUAA4ICDwAwggIKAoICAQDq2Gxc3G6Ubt2c8OSBOmH3LMahptbGzHIOh1Fz\n1OqpIPrk2A2kFXSM4WMTM3TtlqHSFS4SzCzC1yPThIIMTeqjjRtIYYGkD9uDCxnM\n9xyJ5cq0ob0Cn1sCT+n9qhpCYT+MY7I0OqwTPDJqTm9PU2QEhL1UqtA4CLy462WE\nf639lsphjvsIVkiuNuACuHddKwpxBPQo+jEWuDJVJXiGppSnG9razl/M+Yk16MEp\nldMf2fMcBtMmq3rdaxmAPwz8l5pnLm5gEi4MmCY104Axn8/C/97xQorZdhmqwUhZ\nQQLC4loHu5U31Dk/n0BwRR+NdYqFe9+Q9NYgJJ7pprpKNEcB2PdjHNxjZE6YO+9U\nbdpBGkLJLHjCbCfTfLQV8gl20adelYX+2R4lxHbtyha2B0FOINxXPmX2OFfL/TSo\nhWoDQojDhr76g2w0SgcZmEzfSgpRYhDcu9en5OuLihVCkyfB2vVIcyIu/CIE4s7l\nHCWluiygHoQkTZfZ8RBTdxr+Q+SyjmundG6B3Lyh8Qceht2NEMibXWnSEHXDaFH5\nFwhSAQ0Ck8zX8L1T8VD80LjrfaN5ZCXhL/513SD6PrnokjLXwZYQfagDasioglW4\npNLnI5/cc2hFrJ3hkCBUV50mApv4g/AgKw1kVDJXeAOcOmEK1X9QKtVS22wsZpaq\nYxtSNwIDAQABo0IwQDAOBgNVHQ8BAf8EBAMCAAEwDwYDVR0TAQH/BAUwAwEB/zAd\nBgNVHQ4EFgQUW6YRaBr3zIAIopT9d/wmE5xzuYkwDQYJKoZIhvcNAQELBQADggIB\nABl+zmZOqUF24F+vu4l1Isi75cW96QPpfsH6beR+RnZNqOZOx/Oh+ZjbRNCbltia\nJ6NiwImHOkiTM+LdZEg30/fYNUvWhMRF1KOSBPZb6ReaMNnCqaCJSaS6AHBisSUe\nM0+K09I5aPGZq8VkVtdQfOUYK2Uf4W50HndJkpIgdcn8J4RyTRY9rUo4ZHSA5uCR\nDY/Zkw2af4peqpIEBrSPuh72nRFlrFcTxAofzPr+A2cJxWR2i3/WGd10bHg6Mg6C\nV+und3n2dsb8g+XxI64YYOy/NQWjgNhz91BWpcx2j6JMYrcZOn6mkwYaehYqPT0x\nEdyQmjJ54r1Ta0u8tjmPMgXYzIJEDS3x5dj2JJXTGP4xcAMn329/wRdkrAUqi8Wt\ne0oSwm+QMv0o6zUlDId1RbSgG+uonbgAqVZIxuuLYzoYEfxLF8iEFvpy0tu5FO86\ncUJm7EgjkBJ3KFx21j7e3H2wsDLvgYd6dAZC+NFX0GgikPDM2tgI5imQsS6bGLwP\nuF8xa5C6biCqqQ9xeN+u6hwhNuCBgqKjPPkRjZCdqCmm6lZl8tKbuZHzDt9T/D1f\n/WkcoyWB0ZI4f6JI26lAOMdyeXauQ4mRHAg/4lwe94aWxKMGNSMiNNAnwb7k5dhe\nweMF0kI4NIHvoshdmkfiUE+TWGGyFT53uoGUVOqabkFJ\n-----END CERTIFICATE-----"}

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
