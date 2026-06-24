package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"reflect"
)

type Envs struct {
	REDIS_HOST,
	POSTGRES_HOST,
	ADMIN_USER,
	ADMIN_PASSWORD,
	ADMIN_HMAC,
	POSTGRES_PASSWORD,
	POSTGRES_USER,
	POSTGRES_DB string
}

type ReqUserRegister struct {
	User     string
	Password string
	Role     string
	HMAC     string
}

func GetEnvs() *Envs {
	envs := Envs{}
	ref := reflect.ValueOf(&envs).Elem()

	for k := range ref.Fields() {
		env, ok := os.LookupEnv(k.Name)
		if !ok {
			log.Fatalf("no env %q is set", k.Name)
		}
		ref.FieldByName(k.Name).SetString(env)
	}

	return &envs
}

func LoadDefaultErrors() *DefaultErrors {
	f, err := os.ReadFile("errors.json")
	if err != nil {
		log.Fatal("could not load errors.json file")
	}

	var defErrs DefaultErrors
	err = json.Unmarshal(f, &defErrs)
	if err != nil {
		log.Fatalf("failed to marshal default errors on startup: %v", err)
	}

	return &defErrs
}

func (app *App) RegisterAdmin(ctx context.Context) {
	ok, err := app.adminExists(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		_, err := app.userRegister(ctx, &ReqUserRegister{User: app.Envs.ADMIN_USER, Password: app.Envs.ADMIN_PASSWORD, Role: "admin", HMAC: app.Envs.ADMIN_HMAC})
		if err != nil {
			log.Fatalf("failed to register admin user: %v", err)
		}
	}
}

func (app *App) CheckDefault(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		w.WriteHeader(http.StatusGatewayTimeout)
		w.Write(app.DefaultErrors.Timedout)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(app.DefaultErrors.UnexpectedError)
	}
}

func (resp *Response) WriteResponse(req *http.Request, w http.ResponseWriter, app *App) {
	respBody, err := json.Marshal(resp)
	if err != nil {
		ErrLog.Printf("could not marshal reponse to JSON for %s:\n%v", req.URL.Path, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(app.DefaultErrors.UnexpectedError)
	} else {
		w.Write(respBody)
	}
}

func (app *App) WriteNewResponse(msg string, w http.ResponseWriter, req *http.Request) {
	resp := Response{Message: msg}
	resp.WriteResponse(req, w, app)
}
