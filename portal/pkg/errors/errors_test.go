package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	err := New(CodeUserNotFound, "user not found")
	assert.Equal(t, CodeUserNotFound, err.Code)
	assert.Equal(t, "user not found", err.Message)
}

func TestNew_EmptyMessage(t *testing.T) {
	err := New(CodeUserNotFound, "")
	assert.Equal(t, CodeUserNotFound, err.Code)
	assert.Equal(t, "user not found", err.Message) // uses default message
}

func TestWrap(t *testing.T) {
	inner := errors.New("inner error")
	err := Wrap(CodeInternal, "operation failed", inner)
	assert.Equal(t, CodeInternal, err.Code)
	assert.Equal(t, "operation failed", err.Message)
	assert.Equal(t, inner, err.Err)
}

func TestWrapCode(t *testing.T) {
	inner := errors.New("inner error")
	err := WrapCode(CodeUserNotFound, inner)
	assert.Equal(t, CodeUserNotFound, err.Code)
	assert.Equal(t, inner, err.Err)
}

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected string
	}{
		{
			name:     "without inner error",
			err:      New(CodeUserNotFound, "user not found"),
			expected: "user not found",
		},
		{
			name:     "with inner error",
			err:      Wrap(CodeInternal, "operation failed", errors.New("db error")),
			expected: "operation failed: db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := Wrap(CodeInternal, "operation failed", inner)
	assert.Equal(t, inner, err.Unwrap())
}

func TestAppError_Is(t *testing.T) {
	err1 := New(CodeUserNotFound, "user not found")
	err2 := New(CodeUserNotFound, "different message")
	err3 := New(CodeRoleNotFound, "role not found")

	assert.True(t, errors.Is(err1, err2))
	assert.False(t, errors.Is(err1, err3))
}

func TestAsAppError(t *testing.T) {
	appErr := New(CodeUserNotFound, "user not found")

	result, ok := AsAppError(appErr)
	assert.True(t, ok)
	assert.Equal(t, appErr, result)

	_, ok = AsAppError(errors.New("plain error"))
	assert.False(t, ok)
}

func TestIsCode(t *testing.T) {
	appErr := New(CodeUserNotFound, "user not found")
	plainErr := errors.New("plain error")

	assert.True(t, IsCode(appErr, CodeUserNotFound))
	assert.False(t, IsCode(appErr, CodeRoleNotFound))
	assert.False(t, IsCode(plainErr, CodeUserNotFound))
}

func TestAppError_HTTPStatus(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected int
	}{
		{"200 OK", CodeOK, 500}, // default to 500
		{"400 Bad Request", CodeBadRequest, 400},
		{"401 Unauthorized", CodeUnauthorized, 401},
		{"403 Forbidden", CodeForbidden, 403},
		{"404 Not Found", CodeNotFound, 404},
		{"409 Conflict", CodeConflict, 409},
		{"500 Internal", CodeInternal, 500},
		{"1001 Business Code", CodeUserNotFound, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "")
			assert.Equal(t, tt.expected, err.HTTPStatus())
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	assert.Equal(t, CodeNotFound, NotFoundError("not found").Code)
	assert.Equal(t, CodeBadRequest, BadRequestError("bad request").Code)
	assert.Equal(t, CodeUnauthorized, UnauthorizedError("unauthorized").Code)
	assert.Equal(t, CodeForbidden, ForbiddenError("forbidden").Code)
	assert.Equal(t, CodeConflict, ConflictError("conflict").Code)
	assert.Equal(t, CodeInternal, InternalError("internal").Code)
}
