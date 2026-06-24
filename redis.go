package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"runtime"
	"strconv"
	"time"

	"github.com/b-zago/rikami-api/auth"
	"github.com/redis/go-redis/v9"
)

func (app *App) NewRedisClient(ctx context.Context) {
	app.RedisClient = redis.NewClient(&redis.Options{
		Addr:     app.Envs.REDIS_HOST + ":6379",
		Password: "",
		DB:       0,

		// Connection pool — tune for production
		PoolSize:        10 * runtime.NumCPU(),
		MinIdleConns:    10,
		MaxIdleConns:    30,
		ConnMaxIdleTime: 30 * time.Minute,
		ConnMaxLifetime: 1 * time.Hour,

		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,

		MaxRetries:      3,
		MinRetryBackoff: 8 * time.Millisecond,
		MaxRetryBackoff: 512 * time.Millisecond,
	})

	if err := app.RedisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis initial ping error: %v", err)
	}
}

func (app *App) loginUserTokens(ctx context.Context, userID int, tokensJSON []byte) error {
	userKey := "user:" + strconv.Itoa(userID)
	currentTokens, err := app.RedisClient.Get(ctx, userKey).Result()
	if err == redis.Nil {
		err = app.RedisClient.Set(ctx, userKey, tokensJSON, 6*time.Hour).Err()
		if err != nil {
			return err
		}
		return nil

	} else if err != nil {
		return err
	}
	var tokens auth.Tokens
	err = json.Unmarshal([]byte(currentTokens), &tokens)
	if err != nil {
		return err
	}
	err = app.RedisClient.Del(ctx, "token:refresh:"+tokens.Refresh, "token:short:"+tokens.Short).Err()
	if err != nil {
		return err
	}
	err = app.RedisClient.Set(ctx, userKey, tokensJSON, 6*time.Hour).Err()
	if err != nil {
		return err
	}
	return nil
}

func (app *App) setTokens(ctx context.Context, tokens *auth.Tokens, rTokenData []byte) error {
	pipe := app.RedisClient.TxPipeline()
	refreshCmd := pipe.SetNX(ctx, "token:refresh:"+tokens.Refresh, rTokenData, 6*time.Hour)
	shortCmd := pipe.SetNX(ctx, "token:short:"+tokens.Short, tokens.Refresh, 10*time.Minute)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	if refreshCmd.Val() && shortCmd.Val() {
		return nil
	} else {
		return errors.New("some tokens were already set")
	}
}

func (app *App) getUserByShortToken(ctx context.Context, token string) (*RTokenCacheData, error) {
	refToken, err := app.RedisClient.Get(ctx, "token:short:"+token).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return app.getUserByRefreshToken(ctx, refToken)
}

func (app *App) getUserByRefreshToken(ctx context.Context, token string) (*RTokenCacheData, error) {
	userDataStr, err := app.RedisClient.Get(ctx, "token:refresh:"+token).Result()
	if err != nil {
		return nil, err
	}

	var userData RTokenCacheData
	err = json.Unmarshal([]byte(userDataStr), &userData)
	if err != nil {
		return nil, err
	}

	return &userData, nil
}

func (app *App) newShortTokenFromRefresh(ctx context.Context, userID int) (string, error) {
	userKey := "user:" + strconv.Itoa(userID)
	currentTokens, err := app.RedisClient.Get(ctx, userKey).Result()
	if err != nil {
		return "", err
	}

	var tokens auth.Tokens
	err = json.Unmarshal([]byte(currentTokens), &tokens)
	if err != nil {
		return "", err
	}

	// just to ensure the other short token gets cleared
	err = app.RedisClient.Del(ctx, "token:short:"+tokens.Short).Err()
	if err != nil {
		return "", err
	}

	newShortToken, err := auth.NewToken(32)
	if err != nil {
		return "", err
	}

	tokens.Short = newShortToken
	newUserData, err := json.Marshal(&tokens)
	if err != nil {
		return "", nil
	}

	pipe := app.RedisClient.TxPipeline()
	pipe.Set(ctx, userKey, newUserData, redis.KeepTTL)
	pipe.Set(ctx, "token:short:"+tokens.Short, tokens.Refresh, 10*time.Minute)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", err
	}

	return tokens.Short, nil
}
