package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"linknest/server/internal/auth"
	"linknest/server/internal/device"
	"linknest/server/internal/file"
	"linknest/server/internal/middleware"
	"linknest/server/internal/response"
	"linknest/server/internal/task"
	servertransfer "linknest/server/internal/transfer"
	lnwebsocket "linknest/server/internal/websocket"
)

type Dependencies struct {
	Auth      *auth.Service
	Device    *device.Service
	File      *file.Service
	Task      *task.Service
	Transfer  *servertransfer.Service
	WebSocket *lnwebsocket.Handler
	StaticDir string
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	staticServer := http.StripPrefix("/static/", http.FileServer(http.Dir(deps.StaticDir)))

	mux.HandleFunc("/healthz", method(http.MethodGet, healthz))
	mux.HandleFunc("/", serveIndex(deps.StaticDir))
	mux.Handle("/static/", staticServer)
	mux.HandleFunc("/login", servePage(deps.StaticDir, "login.html"))
	mux.HandleFunc("/devices", servePage(deps.StaticDir, "devices.html"))
	mux.HandleFunc("/files", servePage(deps.StaticDir, "files.html"))
	mux.HandleFunc("/tasks", servePage(deps.StaticDir, "tasks.html"))

	mux.HandleFunc("/api/auth/register", method(http.MethodPost, handleRegister(deps.Auth)))
	mux.HandleFunc("/api/auth/login", method(http.MethodPost, handleLogin(deps.Auth)))
	mux.Handle("/api/auth/me", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodGet, handleMe())))
	mux.Handle("/api/auth/delete-account", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodPost, handleDeleteAccount(deps.Auth))))

	mux.Handle("/api/devices/register", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodPost, handleDeviceRegister(deps.Device))))
	mux.Handle("/api/devices", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodGet, handleDeviceList(deps.Device))))

	mux.Handle("/api/files", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodGet, handleFileList(deps.File))))
	mux.Handle("/api/files/init-upload", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodPost, handleInitUpload(deps.File))))
	mux.Handle("/api/files/", middleware.RequireAuth(deps.Auth, http.HandlerFunc(handleFileRoutes(deps.File))))

	mux.Handle("/api/tasks", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodGet, handleTaskList(deps.Task))))
	mux.Handle("/api/uploads/", middleware.RequireAuth(deps.Auth, http.HandlerFunc(handleUploadRoutes(deps.File))))
	mux.Handle("/api/transfers/init", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodPost, handleTransferInit(deps.Transfer))))
	mux.Handle("/api/transfers/validate-token", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodPost, handleTransferValidateToken(deps.Transfer))))
	mux.Handle("/api/transfers", middleware.RequireAuth(deps.Auth, methodHandler(http.MethodGet, handleTransferList(deps.Transfer))))
	mux.Handle("/api/transfers/", middleware.RequireAuth(deps.Auth, http.HandlerFunc(handleTransferRoutes(deps.Transfer))))

	mux.Handle("/ws/devices", middleware.RequireAuth(deps.Auth, deps.WebSocket))

	return middleware.Recover(middleware.RequestLog(mux))
}

func method(allowed string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != allowed {
			response.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		next(w, r)
	}
}

func methodHandler(allowed string, next http.HandlerFunc) http.Handler {
	return method(allowed, next)
}

func healthz(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func serveIndex(staticDir string) http.HandlerFunc {
	indexPath := filepath.Join(staticDir, "index.html")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}

		response.JSON(w, http.StatusOK, map[string]string{
			"name":   "LinkNest V1",
			"status": "server ready",
		})
	}
}

func servePage(staticDir string, filename string) http.HandlerFunc {
	pagePath := filepath.Join(staticDir, filename)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		http.ServeFile(w, r, pagePath)
	}
}

func handleRegister(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input auth.RegisterInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		result, err := service.Register(r.Context(), input)
		if err != nil {
			if errors.Is(err, auth.ErrUserExists) {
				response.Error(w, http.StatusConflict, "AUTH_USER_EXISTS", "username or email already exists")
				return
			}
			response.Error(w, http.StatusBadRequest, "AUTH_REGISTER_FAILED", err.Error())
			return
		}

		response.JSON(w, http.StatusCreated, result)
	}
}

func handleLogin(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input auth.LoginInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		result, err := service.Login(r.Context(), input)
		if err != nil {
			if errors.Is(err, auth.ErrBadCredentials) {
				response.Error(w, http.StatusUnauthorized, "AUTH_BAD_CREDENTIALS", "invalid username or password")
				return
			}
			response.Error(w, http.StatusBadRequest, "AUTH_LOGIN_FAILED", err.Error())
			return
		}

		response.JSON(w, http.StatusOK, result)
	}
}

func handleMe() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}
		response.JSON(w, http.StatusOK, map[string]interface{}{
			"user": user,
		})
	}
}

func handleDeleteAccount(service *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		var input auth.DeleteAccountInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		result, err := service.DeleteAccount(r.Context(), user.ID, input)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrBadCredentials):
				response.Error(w, http.StatusUnauthorized, "AUTH_BAD_CREDENTIALS", "invalid username or password")
			case errors.Is(err, auth.ErrInvalidToken):
				response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			default:
				response.Error(w, http.StatusInternalServerError, "AUTH_DELETE_ACCOUNT_FAILED", err.Error())
			}
			return
		}

		response.JSON(w, http.StatusOK, result)
	}
}

func handleDeviceRegister(service *device.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		var input device.RegisterInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		item, err := service.RegisterOrUpdate(r.Context(), user.ID, input)
		if err != nil {
			response.Error(w, http.StatusBadRequest, "DEVICE_REGISTER_FAILED", err.Error())
			return
		}

		response.JSON(w, http.StatusOK, map[string]interface{}{
			"device_id":  item.DeviceID,
			"registered": true,
		})
	}
}

func handleDeviceList(service *device.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		var (
			items []device.Device
			err   error
		)
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("status")), "online") {
			items, err = service.ListOnlineByUser(r.Context(), user.ID)
		} else {
			items, err = service.ListByUser(r.Context(), user.ID)
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "DEVICE_LIST_FAILED", err.Error())
			return
		}

		response.JSON(w, http.StatusOK, map[string]interface{}{
			"items": items,
		})
	}
}

func handleFileList(service *file.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		items, err := service.ListByUser(r.Context(), user.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "FILE_LIST_FAILED", err.Error())
			return
		}

		response.JSON(w, http.StatusOK, map[string]interface{}{
			"items": items,
		})
	}
}

func handleInitUpload(service *file.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		var input file.InitUploadInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		result, err := service.InitUpload(r.Context(), user.ID, input)
		if err != nil {
			switch {
			case errors.Is(err, file.ErrDeviceNotRegistered):
				response.Error(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "device does not exist or does not belong to current user")
			default:
				response.Error(w, http.StatusBadRequest, "UPLOAD_INIT_FAILED", err.Error())
			}
			return
		}

		response.JSON(w, http.StatusOK, result)
	}
}

func handleTaskList(service *task.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		items, err := service.ListByUser(r.Context(), user.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "TASK_LIST_FAILED", err.Error())
			return
		}

		response.JSON(w, http.StatusOK, map[string]interface{}{
			"items": items,
		})
	}
}

func handleTransferInit(service *servertransfer.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		var input servertransfer.InitInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		result, err := service.Init(r.Context(), user.ID, input)
		if err != nil {
			if errors.Is(err, servertransfer.ErrDeviceNotFound) {
				response.Error(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "source or target device does not exist")
				return
			}
			response.Error(w, http.StatusBadRequest, "TRANSFER_INIT_FAILED", err.Error())
			return
		}
		response.JSON(w, http.StatusOK, result)
	}
}

func handleTransferValidateToken(service *servertransfer.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		var input servertransfer.ValidateTokenInput
		if err := decodeJSON(r, &input); err != nil {
			response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}

		result, err := service.ValidateToken(r.Context(), user.ID, input)
		if err != nil {
			response.Error(w, http.StatusUnauthorized, "P2P_TOKEN_INVALID", "p2p transfer token is invalid")
			return
		}
		response.JSON(w, http.StatusOK, result)
	}
}

func handleTransferList(service *servertransfer.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		items, err := service.ListByUser(r.Context(), user.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "TRANSFER_LIST_FAILED", err.Error())
			return
		}
		response.JSON(w, http.StatusOK, map[string]interface{}{"items": items})
	}
}

func handleTransferRoutes(service *servertransfer.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		transferID, action, ok := splitTail(r.URL.Path, "/api/transfers/")
		if !ok || transferID == "" {
			http.NotFound(w, r)
			return
		}

		switch {
		case r.Method == http.MethodGet && action == "detail":
			task, err := service.Get(r.Context(), user.ID, transferID)
			if err != nil {
				handleTransferError(w, err)
				return
			}
			response.JSON(w, http.StatusOK, task)
			return
		case r.Method == http.MethodPost && action == "probe-result":
			var input servertransfer.ProbeResultInput
			if err := decodeJSON(r, &input); err != nil {
				response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			task, err := service.ReportProbeResult(r.Context(), user.ID, transferID, input)
			if err != nil {
				handleTransferError(w, err)
				return
			}
			response.JSON(w, http.StatusOK, task)
			return
		case r.Method == http.MethodPost && action == "complete":
			var input servertransfer.CompleteInput
			if err := decodeJSON(r, &input); err != nil {
				response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			task, err := service.Complete(r.Context(), user.ID, transferID, input)
			if err != nil {
				handleTransferError(w, err)
				return
			}
			response.JSON(w, http.StatusOK, task)
			return
		case r.Method == http.MethodPost && action == "fallback":
			var input servertransfer.FallbackInput
			if err := decodeJSON(r, &input); err != nil {
				response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			result, err := service.Fallback(r.Context(), user.ID, transferID, input)
			if err != nil {
				handleTransferError(w, err)
				return
			}
			response.JSON(w, http.StatusOK, result)
			return
		case r.Method == http.MethodPost && action == "mark-failed":
			var input struct {
				ErrorCode    string `json:"error_code"`
				ErrorMessage string `json:"error_message"`
			}
			if err := decodeJSON(r, &input); err != nil {
				response.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
				return
			}
			task, err := service.MarkFailed(r.Context(), user.ID, transferID, input.ErrorCode, input.ErrorMessage)
			if err != nil {
				handleTransferError(w, err)
				return
			}
			response.JSON(w, http.StatusOK, task)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}
}

func handleTransferError(w http.ResponseWriter, err error) {
	if errors.Is(err, servertransfer.ErrTransferNotFound) {
		response.Error(w, http.StatusNotFound, "TRANSFER_NOT_FOUND", "transfer task does not exist or does not belong to current user")
		return
	}
	response.Error(w, http.StatusBadRequest, "TRANSFER_FAILED", err.Error())
}

func handleFileRoutes(service *file.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/files/"), "/")
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}

		parts := strings.Split(trimmed, "/")
		fileID := parts[0]
		action := ""
		if len(parts) > 1 {
			action = strings.Join(parts[1:], "/")
		}

		switch {
		case r.Method == http.MethodDelete && action == "":
			if err := service.Delete(r.Context(), user.ID, fileID); err != nil {
				if errors.Is(err, file.ErrFileNotFound) {
					response.Error(w, http.StatusNotFound, "FILE_NOT_FOUND", "file does not exist or does not belong to current user")
					return
				}
				response.Error(w, http.StatusInternalServerError, "FILE_DELETE_FAILED", err.Error())
				return
			}
			response.JSON(w, http.StatusOK, map[string]interface{}{
				"file_id": fileID,
				"deleted": true,
				"status":  "deleted",
			})
			return
		case r.Method == http.MethodGet && action == "download":
			record, err := service.OpenDownload(r.Context(), user.ID, fileID)
			if err != nil {
				switch {
				case errors.Is(err, file.ErrFileNotFound):
					response.Error(w, http.StatusNotFound, "FILE_NOT_FOUND", "file does not exist or does not belong to current user")
				case errors.Is(err, file.ErrFileNotAvailable):
					response.Error(w, http.StatusConflict, "FILE_NOT_AVAILABLE", "file is not available for download")
				default:
					response.Error(w, http.StatusInternalServerError, "FILE_DOWNLOAD_FAILED", err.Error())
				}
				return
			}

			input, err := os.Open(record.StoragePath)
			if err != nil {
				response.Error(w, http.StatusInternalServerError, "FILE_DOWNLOAD_FAILED", err.Error())
				return
			}
			defer input.Close()

			info, err := input.Stat()
			if err != nil {
				response.Error(w, http.StatusInternalServerError, "FILE_DOWNLOAD_FAILED", err.Error())
				return
			}

			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(record.FileName)))
			w.Header().Set("X-File-Hash", record.FileHash)
			if record.MIMEType != "" {
				w.Header().Set("Content-Type", record.MIMEType)
			}
			http.ServeContent(w, r, record.FileName, info.ModTime(), input)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}
}

func handleUploadRoutes(service *file.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			response.Error(w, http.StatusUnauthorized, "AUTH_INVALID_TOKEN", "invalid token")
			return
		}

		uploadID, action, ok := splitTail(r.URL.Path, "/api/uploads/")
		if !ok || uploadID == "" {
			http.NotFound(w, r)
			return
		}

		switch {
		case r.Method == http.MethodGet && action == "missing-chunks":
			result, err := service.GetMissingChunks(r.Context(), user.ID, uploadID)
			if err != nil {
				if errors.Is(err, file.ErrUploadNotFound) {
					response.Error(w, http.StatusNotFound, "UPLOAD_NOT_FOUND", "upload task does not exist or does not belong to current user")
					return
				}
				response.Error(w, http.StatusInternalServerError, "UPLOAD_MISSING_CHUNKS_FAILED", err.Error())
				return
			}
			response.JSON(w, http.StatusOK, result)
			return
		case r.Method == http.MethodPost && action == "complete":
			result, err := service.CompleteUpload(r.Context(), user.ID, uploadID)
			if err != nil {
				switch {
				case errors.Is(err, file.ErrUploadNotFound):
					response.Error(w, http.StatusNotFound, "UPLOAD_NOT_FOUND", "upload task does not exist or does not belong to current user")
				case errors.Is(err, file.ErrMissingChunks):
					response.JSON(w, http.StatusConflict, result)
				case errors.Is(err, file.ErrFileHashMismatch):
					response.Error(w, http.StatusConflict, "UPLOAD_FILE_HASH_MISMATCH", "merged file hash mismatch")
				default:
					response.Error(w, http.StatusInternalServerError, "UPLOAD_COMPLETE_FAILED", err.Error())
				}
				return
			}
			response.JSON(w, http.StatusOK, result)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(action, "chunks/"):
			indexText := strings.TrimPrefix(action, "chunks/")
			chunkIndex, err := strconv.Atoi(indexText)
			if err != nil {
				response.Error(w, http.StatusBadRequest, "BAD_REQUEST", "chunk index must be an integer")
				return
			}

			result, err := service.UploadChunk(r.Context(), user.ID, uploadID, chunkIndex, r.Header.Get("X-Chunk-Hash"), r.Body)
			if err != nil {
				switch {
				case errors.Is(err, file.ErrUploadNotFound):
					response.Error(w, http.StatusNotFound, "UPLOAD_NOT_FOUND", "upload task does not exist or does not belong to current user")
				case errors.Is(err, file.ErrChunkOutOfRange):
					response.Error(w, http.StatusBadRequest, "UPLOAD_CHUNK_OUT_OF_RANGE", "chunk index is out of range")
				case errors.Is(err, file.ErrChunkHashMismatch):
					response.Error(w, http.StatusConflict, "UPLOAD_CHUNK_HASH_MISMATCH", "chunk hash mismatch")
				case errors.Is(err, file.ErrChunkConflict):
					response.Error(w, http.StatusConflict, "UPLOAD_CHUNK_CONFLICT", "chunk already exists with a different hash")
				default:
					response.Error(w, http.StatusInternalServerError, "UPLOAD_CHUNK_FAILED", err.Error())
				}
				return
			}
			response.JSON(w, http.StatusOK, result)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}
}

func decodeJSON(r *http.Request, out interface{}) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func splitTail(pathValue string, prefix string) (string, string, bool) {
	trimmed := strings.TrimPrefix(pathValue, prefix)
	if trimmed == pathValue {
		return "", "", false
	}

	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], strings.Join(parts[1:], "/"), true
}
