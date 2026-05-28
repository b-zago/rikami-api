package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

type EnvConfig struct {
	TargetBranch string
}

func loadEnvConf() *EnvConfig {
	targetBranch, ok := os.LookupEnv("TARGET_BRANCH")
	CheckBool(ok, "TARGET_BRANCH env not set", true)
	return &EnvConfig{
		TargetBranch: targetBranch,
	}
}

type EnvEntry struct {
	EnvName string
	EnvVals map[string]string
}

type SummonRequest struct {
	Vessel     string
	LibVersion string
	Name       string
	Envs       []EnvEntry
}

type AppRequest struct {
	Action  string
	Pattern string
	Param   string
}

type Response struct {
	Message string
}

type AuthedRequest struct {
	UserID string
	User   string
	Body   []byte
}

var ErrLog = log.New(os.Stderr, "ERROR: ", log.LstdFlags)

var EnvConf *EnvConfig

func CheckPrint(err error, fatal bool) {
	if err != nil {
		if fatal {
			log.Fatal(err)
		}
		ErrLog.Println(err)
	}
}

func CheckBool(ok bool, msg string, fatal bool) {
	if !ok {
		if fatal {
			log.Fatal(msg)
		}
		ErrLog.Println(msg)
	}
}

func withAuth(fn func(w http.ResponseWriter, ar *AuthedRequest)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "missing user id", http.StatusBadRequest)
			return
		}

		creds := readEnvs(userID)
		if creds["user"] == "" || creds["token"] == "" {
			http.Error(w, "unknown user", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()

		if !verifyHMAC(body, r.Header.Get("Authorization"), creds["token"]) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		fn(w, &AuthedRequest{UserID: userID, User: creds["user"], Body: body})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func verifyHMAC(body []byte, authHeader string, token string) bool {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "HMAC" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[1]))
}

func handleSummon(w http.ResponseWriter, ar *AuthedRequest) {
	var req SummonRequest
	if err := json.Unmarshal(ar.Body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, TargetRepoSummon(&req, ar.User))
}

func handleApp(w http.ResponseWriter, ar *AuthedRequest) {
	var req AppRequest
	if err := json.Unmarshal(ar.Body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, TargetRepoApp(&req, ar.User))
}

func handleGetCert(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, GetFreshCert())
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, &Response{Message: "Ok!"})
}

func readEnvs(userID string) map[string]string {
	return map[string]string{
		"user":  os.Getenv("USER_" + userID),
		"token": os.Getenv("TOKEN_" + userID),
	}
}

func main() {
	fmt.Println("Starting Rikami controller...")
	loadEnvConf()
	RepoSync()
	TargetRepoSync()
	http.HandleFunc("POST /summon", withAuth(handleSummon))
	http.HandleFunc("POST /app", withAuth(handleApp))
	http.HandleFunc("GET /fetch-cert", handleGetCert)
	http.HandleFunc("GET /health", handleHealth)
	fmt.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("server error:", err)
	}
}
