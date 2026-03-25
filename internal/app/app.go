package app

import (
	"log"
	"net/http"
	"runtime/debug"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/gatekeeper"
	"com.citrus.internalaicopilot/internal/grpcapi"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/rag"

	"google.golang.org/grpc"
)

// App wires the HTTP surface and module dependencies.
type App struct {
	handler           http.Handler
	store             *infra.Store
	gatekeeperUseCase *gatekeeper.UseCase
}

// New bootstraps the application in a single place.
func New(cfg infra.Config) (*App, error) {
	store, err := infra.NewStoreWithOptions(infra.StoreOptions{
		ProjectID:     cfg.FirestoreProjectID,
		EmulatorHost:  cfg.FirestoreEmulatorHost,
		SeedWhenEmpty: true,
		ResetOnStart:  cfg.StoreResetOnStart,
		SeedData:      infra.DefaultSeedData(),
	})
	if err != nil {
		return nil, err
	}

	ragService := rag.NewResolveService(store)
	ragUseCase := rag.NewResolveUseCase(ragService)

	aiService := aiclient.NewAnalyzeService(cfg)
	aiUseCase := aiclient.NewAnalyzeUseCase(aiService)

	outputService := output.NewRenderService()
	outputUseCase := output.NewRenderUseCase(outputService)

	builderQueryService := builder.NewQueryService(store)
	builderAssembleService := builder.NewAssembleService(store)
	builderGraphService := builder.NewGraphService(store, builderQueryService)
	builderTemplateService := builder.NewTemplateService(store, builderQueryService)
	builderConsultUseCase := builder.NewConsultUseCase(store, ragUseCase, aiUseCase, outputUseCase, builderAssembleService, cfg.OpenAIModel)
	builderGraphUseCase := builder.NewGraphUseCase(builderGraphService)
	builderTemplateUseCase := builder.NewTemplateUseCase(builderTemplateService)
	builderHandler := builder.NewAdminHandler(builderGraphUseCase, builderTemplateUseCase)

	gatekeeperGuardService := gatekeeper.NewGuardService(cfg, store)
	gatekeeperUseCase := gatekeeper.NewUseCase(gatekeeperGuardService, builderQueryService, builderConsultUseCase)
	gatekeeperHandler := gatekeeper.NewHandler(gatekeeperUseCase)

	mux := http.NewServeMux()
	gatekeeperHandler.Register(mux)
	builderHandler.Register(mux)

	return &App{
		handler:           withPanicRecovery(withCORS(mux, cfg.CORSAllowedOrigins)),
		store:             store,
		gatekeeperUseCase: gatekeeperUseCase,
	}, nil
}

// Handler returns the fully-wired HTTP handler.
func (a *App) Handler() http.Handler {
	return a.handler
}

// RegisterGRPC wires the app-scoped gRPC services.
func (a *App) RegisterGRPC(registrar grpc.ServiceRegistrar) {
	if a == nil || registrar == nil || a.gatekeeperUseCase == nil {
		return
	}
	grpcapi.Register(registrar, a.gatekeeperUseCase)
}

// Close releases app-scoped resources.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	return a.store.Close()
}

func withPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v\n%s", recovered, debug.Stack())
				infra.WriteError(w, infra.NewError("INTERNAL_SERVER_ERROR", "Internal server error.", http.StatusInternalServerError))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigin, ok := resolveAllowedOrigin(origin, allowedOrigins); ok {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-App-Id")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Max-Age", "3600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func resolveAllowedOrigin(origin string, allowedOrigins []string) (string, bool) {
	for _, allowedOrigin := range allowedOrigins {
		if origin == allowedOrigin {
			return allowedOrigin, true
		}
	}
	return "", false
}
