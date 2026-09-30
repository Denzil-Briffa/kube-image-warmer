package controller

const (
	defaultObjectName            = "default"
	testWarmupPolicyName         = "cluster-images"
	testHealthyWorkerName        = "healthy-worker"
	testNotReadyWorkerName       = "not-ready-worker"
	testTalosNodeName            = "talos-1"
	testBusyBoxImage             = "busybox:1.36.1"
	testRegistryApplicationImage = "registry.example.com/application:v1"
	testPrimaryPullSecretName    = "registry-primary"
	testFallbackPullSecretName   = "registry-fallback"
	testForeignJobName           = "foreign-job"
	testTeamANamespace           = "team-a"
	testTeamBNamespace           = "team-b"
)
