package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/b-zago/rikami-api/auth"
	"github.com/redis/go-redis/v9"
)

type APIHandler func(w http.ResponseWriter, r *http.Request) *ServerError

type ServerError struct {
	Message string
	Status  int
	Err     error
}

type AuthResp struct {
	NeedsRefresh bool `json:"needs_refresh"`
}

type RikamiReqBody string

const RikamiReqBodyKey RikamiReqBody = "body"

func (e *ServerError) Error() string {
	return fmt.Sprintf("%s: %v", e.Message, e.Err)
}

func NewServerError(msg string, status int, err error) *ServerError {
	return &ServerError{Message: msg, Status: status, Err: err}
}

func Logger(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		InfoLog.Printf("endpoint %q got hit\n", req.URL.Path)
		next(w, req)
	}
}

func WithTimeout(seconds int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), time.Duration(seconds)*time.Second)
		defer cancel()
		next(w, req.WithContext(ctx))
	}
}

func Auth(app *App, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		if err != nil {
			ErrLog.Println("body read error: ", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			w.Write(app.DefaultErrors.BadRequest)
			return
		}

		sig := req.Header.Get("x-rikami-signature")
		if sig == "" {
			InfoLog.Println("request with no signature occured")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write(app.DefaultErrors.FailedAuth)
			return
		}

		handlerCtx := context.WithValue(req.Context(), RikamiReqBodyKey, body)
		// check if request came with token first
		token := req.Header.Get("x-rikami-token")
		if token != "" {
			fmt.Printf("token is %q\n", token)
			user, err := app.getUserByShortToken(req.Context(), token)
			if err != nil {
				ErrLog.Println(err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(app.DefaultErrors.UnexpectedError)
				return
			}
			if user == nil {
				InfoLog.Println("requesting refresh token")
				resp := AuthResp{NeedsRefresh: true}
				body, err := json.Marshal(&resp)
				if err != nil {
					ErrLog.Println(err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(app.DefaultErrors.UnexpectedError)
					return
				}
				w.Write(body)
				return
			}
			if !auth.VerifyHMAC([]byte(sig), []byte(user.HMAC), body) {
				InfoLog.Println("failed auth attempt with invalid signature")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write(app.DefaultErrors.FailedAuth)
				return
			}
			// check permissions here for each handler
			next(w, req.WithContext(handlerCtx))
		} else {
			refToken := req.Header.Get("x-rikami-ref-token")
			if refToken != "" {
				fmt.Printf("ref token is %q\n", refToken)
				user, err := app.getUserByRefreshToken(req.Context(), refToken)
				if err == redis.Nil {
					// client needs to login again to generate new set of tokens
					InfoLog.Println("failed auth attempt due to no refresh token found")
					w.WriteHeader(http.StatusUnauthorized)
					w.Write(app.DefaultErrors.FailedAuth)
					return
				} else if err != nil {
					ErrLog.Println(err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(app.DefaultErrors.UnexpectedError)
					return
				}
				newShortToken, err := app.newShortTokenFromRefresh(req.Context(), user.UserID)
				if err != nil {
					ErrLog.Println(err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(app.DefaultErrors.UnexpectedError)
					return
				}
				w.Header().Set("x-rikami-token", newShortToken)
				if !auth.VerifyHMAC([]byte(sig), []byte(user.HMAC), body) {
					InfoLog.Println("failed auth attempt with invalid signature")
					w.WriteHeader(http.StatusUnauthorized)
					w.Write(app.DefaultErrors.FailedAuth)
					return
				}
				// check permission for endpoint here
				next(w, req.WithContext(handlerCtx))
			} else {
				InfoLog.Println("failed auth attempt")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write(app.DefaultErrors.FailedAuth)
				return
			}
		}
	}
}

func HandleErrors(app *App, next APIHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		err := next(w, req)
		if err != nil {

			if errors.Is(err.Err, context.DeadlineExceeded) {
				InfoLog.Printf("request on %q timed out", req.URL.Path)
				w.WriteHeader(http.StatusGatewayTimeout)
				w.Write(app.DefaultErrors.Timedout)
				return
			}

			switch err.Status {
			case http.StatusForbidden:
				InfoLog.Println(err.Message)
				w.WriteHeader(http.StatusForbidden)
				w.Write(app.DefaultErrors.FailedAuth)
			case http.StatusBadRequest:
				InfoLog.Println(err.Message)
				w.WriteHeader(http.StatusBadRequest)
				w.Write(app.DefaultErrors.BadRequest)
			default:
				ErrLog.Println(err.Message)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(app.DefaultErrors.UnexpectedError)
			}
		}
	}
}
