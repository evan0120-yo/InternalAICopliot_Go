// Package builder is the orchestration center of Internal AI Copilot.
//
// Layering:
//
//	UseCase -> Service -> Repository
//
// Responsibilities:
//   - Consult orchestration
//   - Builder/source/template domain rules
//   - Graph save/load
//   - Template CRUD
//   - Prompt assembly
//   - Cross-module coordination with rag, aiclient, and output
//
// Source stays inside builder. Rag stays separate.
package builder
