// Package rag provides retrieval and supplement resolution capabilities.
//
// Layering:
//
//	UseCase -> Service -> Repository
//
// Responsibilities:
//   - Resolve rag configs based on retrievalMode
//   - Hide retrieval details from builder
//   - Preserve future growth for vector search and external retrieval
package rag
