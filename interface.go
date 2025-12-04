package core

import "context"

type Adapter any

// Configurable is called with configurations that match the adapter ID
type Configurable interface {
	ConfigPtr() any
}

// ItemConfigurable is called for named configurations (name string)
// Adapters should capture the name here if they're needed.
// If both a named configuration and adapter configuration
// exists, both will be called.
type ItemConfigurable interface {
	ItemConfigPtr(name string) any
}

// Executor is a generic "do one unit of work" role.
// It is intentionally minimal so it can represent pipeline steps,
// tasks, jobs, commands, etc. in higher-level systems.
type Executor interface {
	Run(ctx context.Context, in ...string) error
	Output(ctx context.Context, in ...string) ([]byte, error)
}

type Builder interface {
	Build(ctx context.Context, in ...string) error // create or first-time apply
}

type Creater interface {
	Create(ctx context.Context, in ...string) error // create or first-time apply
}

type Updater interface {
	Update(ctx context.Context, in ...string) error // idempotent patch / rebuild
}

type Deleter interface {
	Delete(ctx context.Context, in ...string) error // remove everything
}

type Reloader interface {
	Reload(ctx context.Context, in ...string) error // reload / rollout replace
}

// Every adapter must reconcile its resource.
type Lifecycle interface {
	Creater
	Updater
	Deleter
}

// Optional run-time control.
type Starter interface {
	Start(ctx context.Context, in ...string) error // scale >0 / run
}

type Stopper interface {
	Stop(ctx context.Context, in ...string) error // scale 0 / halt
}

type Runner interface {
	Starter
	Stopper
}

type Lister interface {
	List(ctx context.Context) ([]string, error)
}

// Optional
type ListerOf[T any] interface {
	List(ctx context.Context) ([]T, error)
}

type Describer interface {
	Describe(ctx context.Context, name string) (string, error)
}

type Browser interface {
	Lister
	Describer
}

type Configurer interface {
	Configure(ctx context.Context) error // idempotent environment setup
}

// Authentication
type Authenticator interface {
	Login(ctx context.Context) error
}

type Depender interface {
	AddDependency(name string, adapter Adapter)
}

type Contextual interface {
	SetContext(path string) // idempotent environment setup
}

type Uploader interface {
	Upload(ctx context.Context, in ...string) error
}

type Downloader interface {
	Download(ctx context.Context, in ...string) error
}

type Transferer interface {
	Uploader
	Downloader
}

type Filter interface {
	Filter(ctx context.Context, filter string) ([]string, error)
}

type FilterOf[T any] interface {
	Filter(ctx context.Context, filter string) ([]T, error)
}

type Pruner interface {
	// PruneAll(ctx context.Context) error
	Prune(ctx context.Context, filter string) error
}

type Hydrater interface {
	Hydrate(ctx context.Context) error
}
