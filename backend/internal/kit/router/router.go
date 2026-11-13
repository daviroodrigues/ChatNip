package router

func ApiPath(version string, path string) string {
	return "/api/" + version + path
}
