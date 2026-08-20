package errors

// Error code definitions
const (
	CodeOK         = 0
	CodeBadRequest = 400
	CodeUnauthorized = 401
	CodeForbidden = 403
	CodeNotFound = 404
	CodeConflict = 409
	CodeInternal = 500

	// User related error codes (1000-1099)
	CodeUserNotFound      = 1001
	CodeUserExists        = 1002
	CodeInvalidPassword   = 1003
	CodeUsernameExists    = 1004
	CodeEmailExists       = 1005
	CodeLastAdmin         = 1006

	// Role related error codes (1100-1199)
	CodeRoleNotFound      = 1101
	CodeDefaultRoleNotFound = 1102

	// Module related error codes (1200-1299)
	CodeModuleNotFound    = 1201
	CodeModuleHasChildren = 1202

	// Project related error codes (1300-1399)
	CodeProjectNotFound   = 1301

	// Authentication related error codes (1400-1499)
	CodeInvalidToken      = 1401
	CodeTokenExpired      = 1402
	CodeMissingAuthHeader = 1403
)

// Error message mapping
var codeMessages = map[int]string{
	CodeOK:         "success",
	CodeBadRequest: "bad request",
	CodeUnauthorized: "unauthorized",
	CodeForbidden: "forbidden",
	CodeNotFound: "not found",
	CodeConflict: "conflict",
	CodeInternal: "internal server error",

	CodeUserNotFound:      "user not found",
	CodeUserExists:        "user already exists",
	CodeInvalidPassword:   "invalid password",
	CodeUsernameExists:    "username already exists",
	CodeEmailExists:       "email already exists",
	CodeLastAdmin:         "cannot delete the last admin",

	CodeRoleNotFound:      "role not found",
	CodeDefaultRoleNotFound: "default role not found",

	CodeModuleNotFound:    "module not found",
	CodeModuleHasChildren: "module has children, cannot delete",

	CodeProjectNotFound:   "project not found",

	CodeInvalidToken:      "invalid token",
	CodeTokenExpired:      "token expired",
	CodeMissingAuthHeader: "missing authorization header",
}

// GetMessage gets default message for error code
func GetMessage(code int) string {
	if msg, ok := codeMessages[code]; ok {
		return msg
	}
	return "unknown error"
}
