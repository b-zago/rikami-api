package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/b-zago/rikami-api/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Permissions struct {
	Sleep  bool `json:"sleep"`
	Awake  bool `json:"awake"`
	Regen  bool `json:"regen"`
	Update bool `json:"update"`
	Kill   bool `json:"kill"`
	Summon bool `json:"summon"`
	Users  bool `json:"users"`
}

type User struct {
	UserID    int
	RoleID    int
	UserName  string
	Pass      string
	HMACToken string
	Perms     Permissions
}

func (app *App) NewDbPool(ctx context.Context) {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(app.Envs.POSTGRES_USER, app.Envs.POSTGRES_PASSWORD),
		Host:     fmt.Sprintf("%s:5432", app.Envs.POSTGRES_HOST),
		Path:     app.Envs.POSTGRES_DB,
		RawQuery: "sslmode=disable",
	}
	dbURL := u.String()
	// dbURL := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", app.Envs.POSTGRES_USER, app.Envs.POSTGRES_PASSWORD, app.Envs.POSTGRES_HOST, app.Envs.POSTGRES_DB)
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatal("could not parse db url")
	}

	config.MaxConns = 25
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute
	config.MaxConnLifetimeJitter = 5 * time.Minute

	app.DbPool, err = pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatal("could not create db pool")
	}

	// fail fast if DB is unreachable at startup
	if err := app.DbPool.Ping(ctx); err != nil {
		log.Fatal("could not connect to db")
	}
}

func (app *App) getUserByName(ctx context.Context, name string) (*User, error) {
	const query = `
		SELECT u.user_id, u.role_id, u.user_name, u.pass, u.hmac_token,
		       r.sleep, r.awake, r.regen, r."update", r.kill, r.summon, r.users
		FROM users u
		JOIN roles r ON r.role_id = u.role_id
		WHERE u.user_name = $1`

	var u User
	err := app.DbPool.QueryRow(ctx, query, name).Scan(
		&u.UserID, &u.RoleID, &u.UserName, &u.Pass, &u.HMACToken,
		&u.Perms.Sleep, &u.Perms.Awake, &u.Perms.Regen, &u.Perms.Update,
		&u.Perms.Kill, &u.Perms.Summon, &u.Perms.Users,
	)
	if err != nil {
		return nil, fmt.Errorf("query user %q: %w", name, err)
	}
	return &u, nil
}

func (app *App) adminExists(ctx context.Context) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM users u JOIN roles r ON u.role_id = r.role_id WHERE r.role_name = 'admin')`
	var exists bool
	err := app.DbPool.QueryRow(ctx, query).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("adminExists: %w", err)
	}
	fmt.Println("adminExists: ", exists)
	return exists, nil
}

func (app *App) getRoleIDByName(ctx context.Context, roleName string) (int, error) {
	const query = `SELECT role_id FROM roles WHERE role_name = $1`
	var roleID int
	err := app.DbPool.QueryRow(ctx, query, roleName).Scan(&roleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("role %q not found", roleName)
		}
		return 0, fmt.Errorf("getRoleIDByName: %w", err)
	}
	return roleID, nil
}

// in theory phantom writes can occur but it's super unlikely and not critical since only admins can register either way
func (app *App) userRegister(ctx context.Context, req *ReqUserRegister) (string, error) {
	const query = `INSERT INTO users (role_id, user_name, pass, hmac_token) VALUES ($1, $2, $3, $4)`
	if req.HMAC == "" {

		b := make([]byte, 32)
		rand.Read(b)
		req.HMAC = base64.RawStdEncoding.EncodeToString(b)
	}
	roleID, err := app.getRoleIDByName(ctx, req.Role)
	if err != nil {
		return "", err
	}
	password, err := auth.HashPassword(req.Password)
	if err != nil {
		return "", err
	}

	_, err = app.DbPool.Exec(ctx, query, roleID, req.User, password, req.HMAC)
	if err != nil {
		return "", err
	}

	return req.HMAC, nil
}
