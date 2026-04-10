package app

import (
	"context"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"com.citrus.internalaicopilot/internal/aiclient"
	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/gatekeeper"
	"com.citrus.internalaicopilot/internal/grpcapi"
	"com.citrus.internalaicopilot/internal/infra"
	"com.citrus.internalaicopilot/internal/output"
	"com.citrus.internalaicopilot/internal/promptguard"
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
	builderConsultUseCase := builder.NewConsultUseCase(store, ragUseCase, aiUseCase, outputUseCase, builderAssembleService, cfg.ResolvedAIModel())
	builderGraphUseCase := builder.NewGraphUseCase(builderGraphService)
	builderTemplateUseCase := builder.NewTemplateUseCase(builderTemplateService)
	builderHandler := builder.NewAdminHandler(builderGraphUseCase, builderTemplateUseCase)

	gatekeeperGuardService := gatekeeper.NewGuardService(cfg, store)
	promptGuardConfig := promptguard.LoadConfigFromEnv()
	promptGuardService := promptguard.NewService(
		promptGuardConfig,
		promptguard.WithGuardPromptAssembler(func(ctx context.Context, command promptguard.Command) (promptguard.GuardPrompt, error) {
			result, err := builderAssembleService.AssemblePromptGuard(ctx, command.BuilderConfig, command.AppID, command.UserText)
			if err != nil {
				return promptguard.GuardPrompt{}, err
			}
			return promptguard.GuardPrompt{
				Instructions:    result.Instructions,
				UserMessageText: result.UserMessageText,
			}, nil
		}),
		promptguard.WithCloudLLMFunc(func(ctx context.Context, request promptguard.GuardLLMRequest) (promptguard.GuardLLMResponse, error) {
			result, err := aiUseCase.AnalyzeGuard(ctx, aiclient.GuardAnalyzeCommand{
				Route:           aiclient.GuardAnalyzeRouteCloud,
				Model:           request.Model,
				BaseURL:         request.BaseURL,
				APIKey:          request.APIKey,
				Instructions:    request.Instructions,
				UserMessageText: request.UserMessageText,
			})
			if err != nil {
				return promptguard.GuardLLMResponse{}, err
			}
			return promptguard.GuardLLMResponse{
				Status:    result.Status,
				StatusAns: result.StatusAns,
				Reason:    result.Reason,
			}, nil
		}),
		promptguard.WithLocalLLMFunc(func(ctx context.Context, request promptguard.GuardLLMRequest) (promptguard.GuardLLMResponse, error) {
			result, err := aiUseCase.AnalyzeGuard(ctx, aiclient.GuardAnalyzeCommand{
				Route:           aiclient.GuardAnalyzeRouteLocal,
				Model:           request.Model,
				BaseURL:         request.BaseURL,
				APIKey:          request.APIKey,
				Instructions:    request.Instructions,
				UserMessageText: request.UserMessageText,
			})
			if err != nil {
				return promptguard.GuardLLMResponse{}, err
			}
			return promptguard.GuardLLMResponse{
				Status:    result.Status,
				StatusAns: result.StatusAns,
				Reason:    result.Reason,
			}, nil
		}),
	)
	promptGuardUseCase := promptguard.NewEvaluateUseCase(promptGuardService)
	gatekeeperUseCase := gatekeeper.NewUseCase(gatekeeperGuardService, promptGuardUseCase, builderQueryService, builderConsultUseCase)
	gatekeeperHandler := gatekeeper.NewHandler(gatekeeperUseCase)

	mux := http.NewServeMux()
	gatekeeperHandler.Register(mux)
	builderHandler.Register(mux)

	return &App{
		handler:           withRequestLogging(withPanicRecovery(withCORS(mux, cfg.CORSAllowedOrigins))),
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

func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		log.Printf("http request started method=%s path=%s remote_addr=%s origin=%q", r.Method, r.URL.Path, r.RemoteAddr, r.Header.Get("Origin"))

		recorder := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		log.Printf("http request completed method=%s path=%s status=%d bytes=%d duration_ms=%d", r.Method, r.URL.Path, recorder.status(), recorder.bytesWritten, time.Since(startedAt).Milliseconds())
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

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *loggingResponseWriter) Write(body []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	written, err := w.ResponseWriter.Write(body)
	w.bytesWritten += written
	return written, err
}

func (w *loggingResponseWriter) status() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}
