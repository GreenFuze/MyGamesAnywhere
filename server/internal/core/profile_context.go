package core

import "context"

type profileContextKey struct{}

func WithProfile(ctx context.Context, profile *Profile) context.Context {
	if profile == nil {
		return ctx
	}
	return context.WithValue(ctx, profileContextKey{}, profile)
}

func ProfileFromContext(ctx context.Context) (*Profile, bool) {
	profile, ok := ctx.Value(profileContextKey{}).(*Profile)
	return profile, ok && profile != nil
}

func ProfileIDFromContext(ctx context.Context) string {
	if profile, ok := ProfileFromContext(ctx); ok {
		return profile.ID
	}
	return ""
}

func ProfileIsAdmin(ctx context.Context) bool {
	profile, ok := ProfileFromContext(ctx)
	return ok && profile.Role == ProfileRoleAdminPlayer
}
