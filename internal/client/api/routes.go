package api

// routeGroups registers every route group of the client API. Each resource
// registers its routes under Prefix, and the SDK errors its handlers return
// with r.errors.Register, from its own file. Adding a resource is that file
// and one line here.
var routeGroups = []func(r *router, opts *Options){
	systemRoutes,
	desktopRoutes,
}
