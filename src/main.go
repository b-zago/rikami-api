package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

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

func CheckPrint(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func withAuth(fn func(w http.ResponseWriter, ar AuthedRequest)) http.HandlerFunc {
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

		fn(w, AuthedRequest{UserID: userID, User: creds["user"], Body: body})
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

func handleSummon(w http.ResponseWriter, ar AuthedRequest) {
	var req SummonRequest
	if err := json.Unmarshal(ar.Body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, TargetRepoSummon(&req, ar.User))
}

func handleApp(w http.ResponseWriter, ar AuthedRequest) {
	var req AppRequest
	if err := json.Unmarshal(ar.Body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, TargetRepoApp(&req, ar.User))
}

func readEnvs(userID string) map[string]string {
	return map[string]string{
		"user":  os.Getenv("USER_" + userID),
		"token": os.Getenv("TOKEN_" + userID),
	}
}

func main() {
	os.Setenv("USER_1f1b082099b90969", "zago")
	os.Setenv("TOKEN_1f1b082099b90969", "acd7221d4d5b785c321cfdfe9e353e34")
	fmt.Println("Starting Rikami controller...")
	RepoSync()
	TargetRepoSync()
	http.HandleFunc("/summon", withAuth(handleSummon))
	http.HandleFunc("/app", withAuth(handleApp))
	fmt.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("server error:", err)
	}
}
