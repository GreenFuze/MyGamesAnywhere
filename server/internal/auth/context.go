package auth

import "context"

type sessionContextKey struct{}
type profileAccessContextKey struct{}

// ProfileAccess records that the selected profile has passed the common access
// policy. Session is nil only when that profile deliberately has no credential.
// Callers must not treat a browser-supplied profile ID as equivalent evidence.
type ProfileAccess struct {
	ProfileID string
	Session   *Session
}

func WithSession(ctx context.Context, session *Session) context.Context {
	if session == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionContextKey{}, session)
}

func SessionFromContext(ctx context.Context) (*Session, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(*Session)
	return session, ok && session != nil
}

func WithProfileAccess(ctx context.Context, access ProfileAccess) context.Context {
	return context.WithValue(ctx, profileAccessContextKey{}, access)
}

func ProfileAccessFromContext(ctx context.Context) (ProfileAccess, bool) {
	access, ok := ctx.Value(profileAccessContextKey{}).(ProfileAccess)
	return access, ok && access.ProfileID != ""
}
