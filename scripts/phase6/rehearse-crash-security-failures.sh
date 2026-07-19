#!/bin/sh
set -eu

export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase6-gocache}"

# Server metadata, indexing checkpoints/transactions, dataset epochs, and
# Review process/report recovery.
go test -count=1 ./internal/process -run 'TestMetadataAtomicUserOnlyRoundTrip'
go test -count=1 ./internal/indexer -run 'TestAppendChangesOnlyNewTurnAndPinnedRevision|TestChangedPrefixAutomaticallyRebuildsAndWriterLock|TestOversizedEventCountTurnCommitsAtomicallyWithinByteBudget'
go test -count=1 ./internal/storage -run 'TestCanceledNormalizationTransactionLeavesNoPartialRevision|TestDatasetEpochAtomicReplacementAndFailedBuildIsolation|TestCandidateCorruptionCannotReplaceActiveCatalog'
go test -count=1 ./internal/reviews -run 'TestLaunchRequiresConfirmationCapturesThreadAndAcceptsFirstValidReport|TestInvalidMissingCitationAndProcessFailureRemainDiagnosable|TestPlanCoverageUsesPinnedSourceInventoryStates'

# Loopback bootstrap/authentication/origin/host, payload-free diagnostics,
# evidence escaping/chunking/missing sources, prompt scope, and path handling.
go test -count=1 ./internal/server -run 'TestSecurityExchangeStatusAndIdleShutdown|TestExchangeRejectsIncompleteMismatchedAndExtraBootstrap|TestUnauthenticatedAPIRejectedAndNoSecretsInResponses|TestStatusExcludesInvalidPersistedMarkerAndReportsDiagnostic|TestPhase6AutomatedDemoBoundarySmoke'
go test -count=1 ./internal/evidence -run 'TestFingerprintMoveMissingAndBoundedResolution|TestEscapesUnsafeOrSplitBytes'
go test -count=1 ./internal/reviews -run 'TestPlanFreezesCompleteSingleAndTimeScopes'
go test -count=1 ./internal/home ./internal/sources ./internal/hook

# Empty, partial, unsupported, stale capacity, missing evidence, malformed
# report, failed Review, safe rendering, and frozen prompt-injection guidance.
pnpm --filter @codex-inspector/web test
