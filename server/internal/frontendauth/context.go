package frontendauth

import "context"

type principalContextKey struct{}
type auditMetadataContextKey struct{}

type AuditMetadata struct {
	RequestID string
	RemoteIP  string
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok && principal.ClientID != "" && principal.ProfileID != ""
}

func WithAuditMetadata(ctx context.Context, metadata AuditMetadata) context.Context {
	return context.WithValue(ctx, auditMetadataContextKey{}, metadata)
}

func auditMetadataFromContext(ctx context.Context) AuditMetadata {
	metadata, _ := ctx.Value(auditMetadataContextKey{}).(AuditMetadata)
	return metadata
}
