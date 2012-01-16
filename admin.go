package admin

import (
	"errors"
	"launchpad.net/mgo"
	"net/http"
	"reflect"
	"strings"
)

//Admin is an http.Handler for serving up the admin pages
type Admin struct {
	Auth     Authorizer        //If not nil, admin is auth protected.
	Session  *mgo.Session      //The mongo session for managing.
	Renderer Renderer          //If nil, a default renderer is used to render the admin pages.
	Routes   map[string]string //Routes lets you change the url paths. If nil, uses DefaultRoutes.
	Prefix   string            //The path the admin is mounted to in the handler.

	//created on demand
	server      *http.ServeMux
	types       map[string]collectionInfo
	index_cache map[string][]string
	object_id   map[reflect.Type]int
	object_coll map[reflect.Type]string
}

//DefaultRoutes is the mapping of actions to url paths.
var DefaultRoutes = map[string]string{
	"index":  "/",
	"list":   "/list/",
	"update": "/update/",
	"create": "/create/",
	"detail": "/detail/",
	"delete": "/delete/",
}

//useful type because these get made so often
type d map[string]interface{}

//adminHandler is a type representing a handler function on an *Admin.
type adminHandler func(*Admin, http.ResponseWriter, *http.Request)

//routes defines the mapping of type to function for the admin
var routes = map[string]adminHandler{
	"index":  (*Admin).index,
	"list":   (*Admin).list,
	"update": (*Admin).update,
	"create": (*Admin).create,
	"detail": (*Admin).detail,
	"delete": (*Admin).delete,
}

//initializeCache makes values in the admin for caching lookups if they don't yet
//exist.
func (a *Admin) initializeCache() {
	if a.types == nil {
		a.types = make(map[string]collectionInfo)
	}
	if a.object_id == nil {
		a.object_id = make(map[reflect.Type]int)
	}
	if a.object_coll == nil {
		a.object_coll = make(map[reflect.Type]string)
	}
}

//generateMux creates the internal http.ServeMux to dispatch reqeusts to the
//appropriate handler.
func (a *Admin) generateMux() {
	if a.server != nil {
		return
	}
	if a.Routes == nil {
		a.Routes = DefaultRoutes
	}

	required := []string{"index", "list", "update", "create", "detail", "delete"}
	for _, r := range required {
		if _, ex := a.Routes[r]; !ex {
			panic("Route missing: " + r)
		}
	}

	a.server = http.NewServeMux()
	for key, path := range a.Routes {
		r, fn := path, routes[key]
		a.server.Handle(r, http.StripPrefix(r, a.bind(fn)))
	}
}

//generateIndexCache generates the values needed for IndexContext and stores
//them for efficient lookup.
func (a *Admin) generateIndexCache() {
	if a.index_cache != nil {
		return
	}

	a.index_cache = make(map[string][]string)
	for key := range a.types {
		pieces := strings.Split(key, ".")
		a.index_cache[pieces[0]] = append(a.index_cache[pieces[0]], pieces[1])
	}
}

//bind turns an adminHandler into an http.HandlerFunc by closing on the admin
//value on the adminHandler.
func (a *Admin) bind(fn adminHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		fn(a, w, req)
	}
}

//collFor returns the mgo.Collection for the specified database.collection.
func (a *Admin) collFor(dbcoll string) mgo.Collection {
	pieces := strings.Split(dbcoll, ".")
	return a.Session.DB(pieces[0]).C(pieces[1])
}

//ServeHTTP lets *Admin conform to the http.Handler interface for use in web servers.
func (a *Admin) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if a.Renderer == nil {
		a.Renderer = newDefaultRenderer()
	}

	//TODO: use the authorizer

	//ensure a valid database
	if a.Session == nil {
		a.Renderer.InternalError(w, req, errors.New("Mongo session not configured"))
		return
	}

	//strip off the prefix
	req.URL.Path = req.URL.Path[len(a.Prefix):]

	//pass it off to our internal muxer
	a.generateMux()
	a.server.ServeHTTP(w, req)
}
