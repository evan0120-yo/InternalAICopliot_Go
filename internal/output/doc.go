// Package output renders AI responses into final output format.
//
// Layering:
//
//	UseCase -> Service
//
// Responsibilities:
//   - Determine whether to include file
//   - Resolve output format
//   - Render markdown or xlsx
//   - Convert rendered files to base64 payload
package output
