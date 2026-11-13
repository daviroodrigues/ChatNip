package apierrors

import "net/http"

type AppError struct {
	Code    int    `json:"-"`
	Message string `json:"error"`
}

func (e *AppError) Error() string {
	return e.Message
}

func NewBadRequest(message string) *AppError {
	return &AppError{
		Code:    http.StatusBadRequest,
		Message: message,
	}
}

func NewConflict(message string) *AppError {
	return &AppError{
		Code:    http.StatusConflict,
		Message: message,
	}
}

func NewUnauthorized(message string) *AppError {
	return &AppError{
		Code:    http.StatusUnauthorized,
		Message: message,
	}
}

func NewForbidden(message string) *AppError {
	return &AppError{
		Code:    http.StatusForbidden,
		Message: message,
	}
}

func NewNotFound(resource string) *AppError {
	return &AppError{
		Code:    http.StatusNotFound,
		Message: resource + " not found",
	}
}

func NewInternalServerError(message string) *AppError {
	if message == "" {
		message = "internal server error"
	}
	return &AppError{
		Code:    http.StatusInternalServerError,
		Message: message,
	}
}

func NewTooManyRequests(message string) *AppError {
	if message == "" {
		message = "too many requests"
	}
	return &AppError{
		Code:    http.StatusTooManyRequests,
		Message: message,
	}
}
