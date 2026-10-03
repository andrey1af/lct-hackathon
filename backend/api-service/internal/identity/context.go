package identity

import "context"

type Principal struct {
	UserID    string
	CompanyID string
	Role      string
	Active    bool
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)

	return p, ok && p.UserID != "" && p.CompanyID != "" && p.Active
}
