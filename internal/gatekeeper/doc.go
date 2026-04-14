// Package gatekeeper is the HTTP entry point for Internal AI Copilot.
//
// Layering:
//
//	Handler -> UseCase -> Service
//
// Responsibilities:
//   - Handle GET /api/builders
//   - Handle POST /api/consult
//   - Handle POST /api/profile-consult
//   - Handle POST /api/line-task-consult
//   - Handle GET /api/external/builders
//   - Handle POST /api/external/consult
//   - Parse multipart requests
//   - Parse strict JSON requests
//   - Resolve client IP
//   - Validate incoming consult request
//   - Forward validated requests into builder use cases
package gatekeeper
