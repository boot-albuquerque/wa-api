package stdio

// Rotas de administração de usuários (`admin.users.*`).

var adminStaticRoutes = map[string]staticRoute{
	"admin.users.add":  {httpMethod: "POST", httpPath: "/admin/users"},
	"admin.users.list": {httpMethod: "GET", httpPath: "/admin/users"},
}

var adminDynamicRoutes = map[string]dynamicRoute{
	"admin.users.get":         {httpMethod: "GET", buildPath: adminUserPath},
	"admin.users.delete":      {httpMethod: "DELETE", buildPath: adminUserPath},
	"admin.users.edit":        {httpMethod: "PUT", buildPath: adminUserPath},
	"admin.users.delete.full": {httpMethod: "DELETE", buildPath: adminUserFullPath},
}

func adminUserPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	userId, ok := ss.stringParam(req, "userId")
	if !ok {
		return "", false
	}
	return "/admin/users/" + userId, true
}

func adminUserFullPath(ss *Server, req *JSONRpcRequest) (string, bool) {
	path, ok := adminUserPath(ss, req)
	if !ok {
		return "", false
	}
	return path + "/full", true
}
