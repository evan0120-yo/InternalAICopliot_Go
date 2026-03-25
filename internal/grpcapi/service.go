package grpcapi

import (
	"context"
	"encoding/base64"
	"net"
	"strings"

	"com.citrus.internalaicopilot/internal/builder"
	"com.citrus.internalaicopilot/internal/gatekeeper"
	"com.citrus.internalaicopilot/internal/grpcapi/pb"
	"com.citrus.internalaicopilot/internal/infra"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Service adapts the gatekeeper use case to gRPC transport.
type Service struct {
	grpcpb.UnimplementedIntegrationServiceServer
	useCase *gatekeeper.UseCase
}

// New builds the gRPC integration service.
func New(useCase *gatekeeper.UseCase) *Service {
	return &Service{useCase: useCase}
}

// Register wires the integration service into a gRPC registrar.
func Register(registrar grpc.ServiceRegistrar, useCase *gatekeeper.UseCase) {
	grpcpb.RegisterIntegrationServiceServer(registrar, New(useCase))
}

// ListBuilders returns active builders for public or app-scoped callers.
func (s *Service) ListBuilders(ctx context.Context, request *grpcpb.ListBuildersRequest) (*grpcpb.ListBuildersResponse, error) {
	appID := strings.TrimSpace(request.GetAppId())

	var (
		items []builder.BuilderSummary
		err   error
	)
	if appID == "" {
		items, err = s.useCase.ListBuilders(ctx)
	} else {
		items, err = s.useCase.ListExternalBuilders(ctx, appID)
	}
	if err != nil {
		return nil, asGRPCError(err)
	}

	response := &grpcpb.ListBuildersResponse{
		Builders: make([]*grpcpb.BuilderSummary, 0, len(items)),
	}
	for _, item := range items {
		builder := &grpcpb.BuilderSummary{
			BuilderId:   int32(item.BuilderID),
			BuilderCode: item.BuilderCode,
			GroupLabel:  item.GroupLabel,
			Name:        item.Name,
			Description: item.Description,
			IncludeFile: item.IncludeFile,
		}
		if item.GroupKey != nil {
			builder.GroupKey = item.GroupKey
		}
		if item.DefaultOutputFormat != nil {
			builder.DefaultOutputFormat = item.DefaultOutputFormat
		}
		response.Builders = append(response.Builders, builder)
	}
	return response, nil
}

// Consult forwards consult requests into the existing gatekeeper flow.
func (s *Service) Consult(ctx context.Context, request *grpcpb.ConsultRequest) (*grpcpb.ConsultResponse, error) {
	attachments := make([]infra.Attachment, 0, len(request.GetAttachments()))
	for _, item := range request.GetAttachments() {
		attachments = append(attachments, infra.Attachment{
			FileName:    item.GetFileName(),
			ContentType: item.GetContentType(),
			Data:        item.GetData(),
		})
	}

	clientIP := resolveClientIP(ctx, request.GetClientIp())
	appID := strings.TrimSpace(request.GetAppId())

	var (
		result infra.ConsultBusinessResponse
		err    error
	)
	if appID == "" {
		result, err = s.useCase.Consult(ctx, "", int(request.GetBuilderId()), request.GetText(), request.GetOutputFormat(), attachments, clientIP)
	} else {
		result, err = s.useCase.ExternalConsult(ctx, appID, int(request.GetBuilderId()), request.GetText(), request.GetOutputFormat(), attachments, clientIP)
	}
	if err != nil {
		return nil, asGRPCError(err)
	}

	response := &grpcpb.ConsultResponse{
		Status:    result.Status,
		StatusAns: result.StatusAns,
		Response:  result.Response,
	}
	if result.File != nil {
		fileBytes, decodeErr := base64.StdEncoding.DecodeString(result.File.Base64)
		if decodeErr != nil {
			return nil, asGRPCError(infra.NewError("INVALID_FILE_PAYLOAD", "Rendered file payload could not be decoded.", 500))
		}
		response.File = &grpcpb.FilePayload{
			FileName:    result.File.FileName,
			ContentType: result.File.ContentType,
			Data:        fileBytes,
		}
	}
	return response, nil
}

// ProfileConsult forwards structured profile-analysis requests into the gatekeeper flow.
func (s *Service) ProfileConsult(ctx context.Context, request *grpcpb.ProfileConsultRequest) (*grpcpb.ProfileConsultResponse, error) {
	clientIP := resolveClientIP(ctx, request.GetClientIp())
	result, err := s.useCase.ProfileConsult(
		ctx,
		strings.TrimSpace(request.GetAppId()),
		int(request.GetBuilderId()),
		request.GetAnalysisModules(),
		toSubjectProfile(request.GetSubjectProfile()),
		request.GetText(),
		clientIP,
	)
	if err != nil {
		return nil, asGRPCError(err)
	}

	return &grpcpb.ProfileConsultResponse{
		Status:    result.Status,
		StatusAns: result.StatusAns,
		Response:  result.Response,
	}, nil
}

func resolveClientIP(ctx context.Context, clientIP string) string {
	if trimmed := strings.TrimSpace(clientIP); trimmed != "" {
		return trimmed
	}
	peerInfo, ok := peer.FromContext(ctx)
	if ok && peerInfo.Addr != nil {
		host, _, err := net.SplitHostPort(peerInfo.Addr.String())
		if err == nil && host != "" {
			return host
		}
		if value := strings.TrimSpace(peerInfo.Addr.String()); value != "" {
			return value
		}
	}
	return "grpc"
}

func asGRPCError(err error) error {
	businessErr := infra.AsBusinessError(err)
	if businessErr == nil {
		return nil
	}

	code := codes.Internal
	switch businessErr.HTTPStatus {
	case 400:
		code = codes.InvalidArgument
	case 401:
		code = codes.Unauthenticated
	case 403:
		code = codes.PermissionDenied
	case 404:
		code = codes.NotFound
	case 409:
		code = codes.AlreadyExists
	case 413:
		code = codes.ResourceExhausted
	case 429:
		code = codes.ResourceExhausted
	case 499:
		code = codes.Canceled
	case 500:
		code = codes.Internal
	case 501:
		code = codes.Unimplemented
	case 502, 503:
		code = codes.Unavailable
	case 504:
		code = codes.DeadlineExceeded
	}

	st := status.New(code, businessErr.Message)
	withDetails, detailErr := st.WithDetails(&errdetails.ErrorInfo{Reason: businessErr.Code})
	if detailErr == nil {
		return withDetails.Err()
	}
	return st.Err()
}

func toSubjectProfile(profile *grpcpb.SubjectProfile) *builder.SubjectProfile {
	if profile == nil {
		return nil
	}

	modulePayloads := make([]builder.SubjectModulePayload, 0, len(profile.GetModulePayloads()))
	for _, payload := range profile.GetModulePayloads() {
		facts := make([]builder.SubjectFact, 0, len(payload.GetFacts()))
		for _, fact := range payload.GetFacts() {
			facts = append(facts, builder.SubjectFact{
				FactKey: fact.GetFactKey(),
				Values:  append([]string(nil), fact.GetValues()...),
			})
		}
		modulePayloads = append(modulePayloads, builder.SubjectModulePayload{
			ModuleKey:     payload.GetModuleKey(),
			TheoryVersion: cloneOptionalString(payload.TheoryVersion),
			Facts:         facts,
		})
	}

	return &builder.SubjectProfile{
		SubjectID:      profile.GetSubjectId(),
		ModulePayloads: modulePayloads,
	}
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
