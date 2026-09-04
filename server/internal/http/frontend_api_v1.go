package http

import (
	"net/http"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// The scoped Frontend API is the only surface an external frontend — Playnite,
// LaunchBox, Pegasus — can reach. It is deliberately a projection of the very
// handlers the management console already uses: "what does this profile own"
// cannot drift between two implementations if there is only one.
//
// What differs is how the caller proved who they are. The console presents a
// profile session cookie; a frontend presents a bearer token that names exactly
// one profile and carries an explicit set of scopes. RequireFrontendAPIClient
// resolves that token to a profile and puts it in the request context, which is
// all these handlers read, so they need no changes to work over either.
//
// What a frontend token deliberately cannot do is anything behind
// RequireAdminProfile. That middleware reads the session-access context, which
// the bearer path never populates, so administration stays a console operation
// even where a shared handler is reachable by both.
//
// Runtime artifacts are absent on purpose. ADR-0047 scopes frontend permissions
// to catalog read, media read, content read, cache preparation and management,
// and gates runtime bytes on per-artifact licence and compliance state instead.
// The old capability response advertised "runtime-artifacts" anyway; serving
// them here would have made the advertisement true by widening the permission
// model, which is not this ticket's decision to make.

// Feature names are the vocabulary a frontend negotiates against. They are part
// of the response contract, so they are named once here.
const (
	featureCapabilityDiscovery = "capability-discovery"
	featureCatalogProjection   = "catalog-projection"
	featureMetadataMedia       = "metadata-media"
	featureContentDelivery     = "content-delivery"
	featureCachePreparation    = "cache-preparation"
)

const frontendAPIV1Prefix = "/api"

// FrontendAPIFeature is one negotiable capability: what it is called, the scope
// a client needs for it, and the endpoints that scope unlocks.
type FrontendAPIFeature struct {
	Name      string             `json:"name"`
	Scope     frontendauth.Scope `json:"scope,omitempty"`
	Endpoints []string           `json:"endpoints"`
}

// frontendAPIRoute is one route on the scoped Frontend API.
//
// The handler is resolved from the builder rather than stored directly so that
// a single table can serve both the live router and the nil-builder router used
// for OpenAPI discovery. A resolver returning nil means the server was built
// without that controller, and the route is simply not registered.
type frontendAPIRoute struct {
	Method  string
	Pattern string
	Feature string
	Scope   frontendauth.Scope

	// Bounded marks a route the standard request timeout may apply to. Content
	// and media stream arbitrarily large files over slow links, so they are
	// registered without one, matching the session-authenticated routes.
	Bounded bool

	resolve func(*RouteBuilder) http.HandlerFunc
}

// frontendAPIV1Routes is the whole scoped surface, in one reviewable place.
// Adding a route here without a scope is a compile-time-visible mistake and a
// test failure: TestEveryFrontendAPIRouteFailsClosed walks the built router and
// insists every path under the prefix refuses an unauthorized caller.
func frontendAPIV1Routes() []frontendAPIRoute {
	return []frontendAPIRoute{
		// Catalog projection: the library as this profile owns it, plus the
		// entitlement and availability evidence recorded by MGA-118 — which is
		// how a frontend tells "must be purchased" from "no longer available".
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/games",
			Feature: featureCatalogProjection, Scope: frontendauth.ScopeCatalogRead, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return gameHandler(b, listGamesOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/games/{id}",
			Feature: featureCatalogProjection, Scope: frontendauth.ScopeCatalogRead, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return gameHandler(b, getGameOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/games/{id}/detail",
			Feature: featureCatalogProjection, Scope: frontendauth.ScopeCatalogRead, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return gameHandler(b, getGameDetailOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/catalog/offers",
			Feature: featureCatalogProjection, Scope: frontendauth.ScopeCatalogRead, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return catalogHandler(b, listOffersOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/catalog/offers/{offer_id}",
			Feature: featureCatalogProjection, Scope: frontendauth.ScopeCatalogRead, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return catalogHandler(b, getOfferOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/catalog/offers/{offer_id}/history",
			Feature: featureCatalogProjection, Scope: frontendauth.ScopeCatalogRead, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return catalogHandler(b, listOfferHistoryOperation) },
		},

		// Metadata and media. HEAD is registered explicitly because a frontend
		// caches cover art and asks for size and ETag before spending bandwidth.
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/media/{assetID}",
			Feature: featureMetadataMedia, Scope: frontendauth.ScopeMetadataRead,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return mediaHandler(b) },
		},
		{
			Method: http.MethodHead, Pattern: "/frontend/v1/media/{assetID}",
			Feature: featureMetadataMedia, Scope: frontendauth.ScopeMetadataRead,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return mediaHandler(b) },
		},

		// Content delivery: the manifest names stable opaque file ids, and the
		// file route serves them through http.ServeContent, so Range, HEAD and
		// If-None-Match behave identically to the session-authenticated route.
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/content/copies/{copy_id}/manifest",
			Feature: featureContentDelivery, Scope: frontendauth.ScopeContentRead,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return contentHandler(b, manifestOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/content/copies/{copy_id}/files/{file_id}",
			Feature: featureContentDelivery, Scope: frontendauth.ScopeContentRead,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return contentHandler(b, fileOperation) },
		},
		{
			Method: http.MethodHead, Pattern: "/frontend/v1/content/copies/{copy_id}/files/{file_id}",
			Feature: featureContentDelivery, Scope: frontendauth.ScopeContentRead,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return contentHandler(b, fileOperation) },
		},

		// Cache preparation, for sources whose bytes do not exist locally until
		// the server materializes them. Separate from content.read because it
		// consumes server storage and a read-only frontend should not trigger it.
		{
			Method: http.MethodPost, Pattern: "/frontend/v1/content/copies/{copy_id}/materializations",
			Feature: featureCachePreparation, Scope: frontendauth.ScopeContentPrepare,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return contentHandler(b, prepareOperation) },
		},
		{
			Method: http.MethodGet, Pattern: "/frontend/v1/content/materializations/{job_id}",
			Feature: featureCachePreparation, Scope: frontendauth.ScopeContentPrepare, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return contentHandler(b, materializationOperation) },
		},
		{
			Method: http.MethodPost, Pattern: "/frontend/v1/content/materializations/{job_id}/cancel",
			Feature: featureCachePreparation, Scope: frontendauth.ScopeContentPrepare, Bounded: true,
			resolve: func(b *RouteBuilder) http.HandlerFunc { return contentHandler(b, cancelMaterializationOperation) },
		},
	}
}

// The operation selectors keep the nil-controller check in one place per
// controller instead of repeating it in fourteen closures.
type (
	gameOperation    func(*GameController) http.HandlerFunc
	catalogOperation func(*CatalogController) http.HandlerFunc
	contentOperation func(*ContentController) http.HandlerFunc
)

func listGamesOperation(c *GameController) http.HandlerFunc     { return c.ListGames }
func getGameOperation(c *GameController) http.HandlerFunc       { return c.Get }
func getGameDetailOperation(c *GameController) http.HandlerFunc { return c.GetDetail }

func listOffersOperation(c *CatalogController) http.HandlerFunc       { return c.ListOffers }
func getOfferOperation(c *CatalogController) http.HandlerFunc         { return c.GetOffer }
func listOfferHistoryOperation(c *CatalogController) http.HandlerFunc { return c.ListHistory }

func manifestOperation(c *ContentController) http.HandlerFunc { return c.Manifest }
func fileOperation(c *ContentController) http.HandlerFunc     { return c.File }
func prepareOperation(c *ContentController) http.HandlerFunc  { return c.Prepare }
func materializationOperation(c *ContentController) http.HandlerFunc {
	return c.GetMaterialization
}
func cancelMaterializationOperation(c *ContentController) http.HandlerFunc {
	return c.CancelMaterialization
}

func gameHandler(b *RouteBuilder, operation gameOperation) http.HandlerFunc {
	if b == nil || b.GameCtrl == nil {
		return nil
	}
	return operation(b.GameCtrl)
}

func catalogHandler(b *RouteBuilder, operation catalogOperation) http.HandlerFunc {
	if b == nil || b.CatalogCtrl == nil {
		return nil
	}
	return operation(b.CatalogCtrl)
}

func contentHandler(b *RouteBuilder, operation contentOperation) http.HandlerFunc {
	if b == nil || b.ContentCtrl == nil {
		return nil
	}
	return operation(b.ContentCtrl)
}

func mediaHandler(b *RouteBuilder) http.HandlerFunc {
	if b == nil || b.MediaCtrl == nil {
		return nil
	}
	return b.MediaCtrl.ServeMedia
}

// registerFrontendAPIV1 mounts the scoped surface and tells the capability
// endpoint what it actually mounted, so discovery cannot promise a route that
// is not there.
func registerFrontendAPIV1(api chi.Router, b *RouteBuilder, middlewareTimeout time.Duration) {
	routes := frontendAPIV1Routes()

	// Capability discovery needs a valid token but no particular scope: a client
	// must be able to find out what it is missing without already having it.
	api.With(RequireFrontendAPIClient(b.FrontendAPIClientSvc, b.ProfileRepo)).
		Get("/frontend/v1/capabilities", b.FrontendAPIClientCtrl.Capabilities)

	mounted := make([]frontendAPIRoute, 0, len(routes))
	for _, route := range routes {
		handler := route.resolve(b)
		if handler == nil {
			continue
		}
		guarded := api.With(RequireFrontendAPIClient(b.FrontendAPIClientSvc, b.ProfileRepo, route.Scope))
		if route.Bounded && middlewareTimeout > 0 {
			guarded = guarded.With(middleware.Timeout(middlewareTimeout))
		}
		guarded.Method(route.Method, route.Pattern, handler)
		mounted = append(mounted, route)
	}

	b.FrontendAPIClientCtrl.SetFeatureCatalog(frontendAPIFeatures(mounted))
}

// registerFrontendAPIV1Discovery registers the same paths with no-op handlers
// for the nil-builder router that generates the OpenAPI document. The paths
// must match registerFrontendAPIV1 exactly or the committed spec drifts.
func registerFrontendAPIV1Discovery(api chi.Router) {
	api.Get("/frontend/v1/capabilities", noopHandler())
	for _, route := range frontendAPIV1Routes() {
		api.Method(route.Method, route.Pattern, noopHandler())
	}
}

// frontendAPIFeatures groups mounted routes into the features a client
// negotiates against, preserving table order so the response is stable.
func frontendAPIFeatures(routes []frontendAPIRoute) []FrontendAPIFeature {
	features := []FrontendAPIFeature{{
		Name:      featureCapabilityDiscovery,
		Endpoints: []string{frontendAPIV1Prefix + "/frontend/v1/capabilities"},
	}}
	index := map[string]int{featureCapabilityDiscovery: 0}

	for _, route := range routes {
		endpoint := frontendAPIV1Prefix + route.Pattern
		position, known := index[route.Feature]
		if !known {
			index[route.Feature] = len(features)
			features = append(features, FrontendAPIFeature{
				Name: route.Feature, Scope: route.Scope, Endpoints: []string{endpoint},
			})
			continue
		}
		if !containsEndpoint(features[position].Endpoints, endpoint) {
			features[position].Endpoints = append(features[position].Endpoints, endpoint)
		}
	}
	return features
}

func containsEndpoint(endpoints []string, candidate string) bool {
	for _, endpoint := range endpoints {
		if endpoint == candidate {
			return true
		}
	}
	return false
}
