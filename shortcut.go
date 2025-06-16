package core

var (
	NewBuilderAdapter       = NewAdapterAs[Builder]
	NewCreaterAdapter       = NewAdapterAs[Creater]
	NewUpdaterAdapter       = NewAdapterAs[Updater]
	NewDeleterAdapter       = NewAdapterAs[Deleter]
	NewLifecycleAdapter     = NewAdapterAs[Lifecycle]
	NewStarterAdapter       = NewAdapterAs[Starter]
	NewStopperAdapter       = NewAdapterAs[Stopper]
	NewRunnerAdapter        = NewAdapterAs[Runner]
	NewListerAdapter        = NewAdapterAs[Lister]
	NewDescriberAdapter     = NewAdapterAs[Describer]
	NewBrowserAdapter       = NewAdapterAs[Browser]
	NewAuthenticatorAdapter = NewAdapterAs[Authenticator]
	NewConfigurerAdapter    = NewAdapterAs[Configurer]
	NewUploaderAdapter      = NewAdapterAs[Uploader]
	NewDownloaderAdapter    = NewAdapterAs[Downloader]
	NewTransfererAdapter    = NewAdapterAs[Transferer]
	NewFilterAdapter        = NewAdapterAs[Filter]
	NewPrunerAdapter        = NewAdapterAs[Pruner]
)
