// Package aiclient handles communication with live AI providers plus preview/mock flows.
//
// Layering:
//
//	UseCase -> Service
//
// Responsibilities:
//   - Accept assembled instructions, user text, and attachments
//   - Resolve preview / mock / live execution mode
//   - Route live requests to OpenAI or Gemma providers
//   - Upload attachments when one provider requires it
//   - Parse structured output
//   - Map provider errors to business errors
package aiclient
