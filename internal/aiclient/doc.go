// Package aiclient handles communication with the OpenAI Responses API.
//
// Layering:
//
//	UseCase -> Service
//
// Responsibilities:
//   - Accept assembled instructions, user text, and attachments
//   - Upload attachments to OpenAI Files API
//   - Call OpenAI Responses API
//   - Parse structured output
//   - Map OpenAI errors to business errors
package aiclient
