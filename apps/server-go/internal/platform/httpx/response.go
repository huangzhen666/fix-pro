package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Response struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"requestId"`
}
type Error struct {
	Code    string
	Message string
	Status  int
	Cause   error
}

func (e *Error) Error() string              { return e.Code + ": " + e.Message }
func E(code, msg string, status int) *Error { return &Error{Code: code, Message: msg, Status: status} }

func Success(w http.ResponseWriter, r *http.Request, data any) {
	write(w, http.StatusOK, Response{Code: "OK", Message: "success", Data: data, RequestID: RequestID(r.Context())})
}
func Failure(w http.ResponseWriter, r *http.Request, err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = E("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError)
	}
	write(w, e.Status, Response{Code: e.Code, Message: e.Message, Data: nil, RequestID: RequestID(r.Context())})
}
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return E("VALIDATION_ERROR", "请求参数格式错误", http.StatusBadRequest)
	}
	return nil
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
