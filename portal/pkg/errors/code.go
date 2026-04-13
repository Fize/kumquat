package errors

// 错误码定义
const (
	CodeOK         = 0
	CodeBadRequest = 400
	CodeUnauthorized = 401
	CodeForbidden = 403
	CodeNotFound = 404
	CodeConflict = 409
	CodeInternal = 500

	// 用户相关错误码 (1000-1099)
	CodeUserNotFound      = 1001
	CodeUserExists        = 1002
	CodeInvalidPassword   = 1003
	CodeUsernameExists    = 1004
	CodeEmailExists       = 1005
	CodeLastAdmin         = 1006

	// 角色相关错误码 (1100-1199)
	CodeRoleNotFound      = 1101
	CodeDefaultRoleNotFound = 1102

	// 模块相关错误码 (1200-1299)
	CodeModuleNotFound    = 1201
	CodeModuleHasChildren = 1202

	// 项目相关错误码 (1300-1399)
	CodeProjectNotFound   = 1301

	// 认证相关错误码 (1400-1499)
	CodeInvalidToken      = 1401
	CodeTokenExpired      = 1402
	CodeMissingAuthHeader = 1403
)

// 错误消息映射
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

// GetMessage 获取错误码对应的默认消息
func GetMessage(code int) string {
	if msg, ok := codeMessages[code]; ok {
		return msg
	}
	return "unknown error"
}
