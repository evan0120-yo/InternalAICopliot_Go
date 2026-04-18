// Package cloud_run contains deployment artifacts for Google Cloud Run.
// Dockerfile and related configs live here.
// Build context must be the project root (InternalAICopliot/Backend/Go/).
//
// Build:
//   docker build -f deployee/cloud_run/Dockerfile .
//
// Deploy:
//   gcloud run deploy internal-ai-copilot --source . --region asia-east1
//
// Serves HTTP and gRPC on the same PORT via h2c (HTTP/2 cleartext).
// Cloud Run terminates TLS; the container sees plain HTTP/2.
package cloud_run
