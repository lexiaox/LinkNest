package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"linknest/server/internal/auth"
	"linknest/server/internal/response"
)

type contextKey string

const userContextKey contextKey = "current_user"

func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("http request method=%s path=%s duration_ms=%d", r.Method, r.URL.Path, time.Since(start).Milliseconds())
	})
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v", recovered)
				response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func RequireAuth(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.ExtractBearerToken(r.Header.Get("Authorization"))
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		user, err := service.ParseToken(token)
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CurrentUser(ctx context.Context) (auth.User, bool) {
	value := ctx.Value(userContextKey)
	if value == nil {
		return auth.User{}, false
	}
	user, ok := value.(auth.User)
	return user, ok
}
