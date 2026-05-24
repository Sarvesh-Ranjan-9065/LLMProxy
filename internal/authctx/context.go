package authctx

import "context"

type contextKey string

const (
	apiKeyKey contextKey = "api_key"
	ownerKey  contextKey = "api_key_owner"
	roleKey   contextKey = "api_key_role"
	tierKey   contextKey = "api_key_tier"
)

func WithAuth(ctx context.Context, apiKey, owner, role, tier string) context.Context {
	ctx = WithAPIKey(ctx, apiKey)
	ctx = WithOwner(ctx, owner)
	ctx = WithRole(ctx, role)
	ctx = WithTier(ctx, tier)
	return ctx
}

func WithAPIKey(ctx context.Context, apiKey string) context.Context {
	return context.WithValue(ctx, apiKeyKey, apiKey)
}

func WithOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerKey, owner)
}

func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleKey, role)
}

func WithTier(ctx context.Context, tier string) context.Context {
	return context.WithValue(ctx, tierKey, tier)
}

func GetAPIKey(ctx context.Context) string {
	if key, ok := ctx.Value(apiKeyKey).(string); ok {
		return key
	}
	return "unknown"
}

func GetOwner(ctx context.Context) string {
	if owner, ok := ctx.Value(ownerKey).(string); ok {
		return owner
	}
	return "unknown"
}

func GetRole(ctx context.Context) string {
	if role, ok := ctx.Value(roleKey).(string); ok {
		return role
	}
	return ""
}

func GetTier(ctx context.Context) string {
	if tier, ok := ctx.Value(tierKey).(string); ok {
		return tier
	}
	return ""
}
