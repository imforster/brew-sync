# Bugfix Requirements Document

## Introduction

The `ComputeDiff` function in `internal/diff/engine.go` misclassifies packages when the same name exists as different types (formula vs cask). The function uses a single `seen` map across both formulae and casks, but Homebrew allows the same name to exist as both a formula and a cask simultaneously. This causes two distinct failures: duplicate entries in the ToRemove set (violating completeness), and incorrect ToInstall classification for packages that are installed locally under a different type (violating soundness). Property-based tests originally caught these failures, but the test generators were subsequently modified to enforce globally unique names — masking the engine bug rather than exposing it.

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN a package name "X" exists in both local formulae and local casks (but not in the manifest) THEN the system adds "X" to the ToRemove set twice, because the single `seen` map does not distinguish between formula and cask entries during the Step 4 local walk

1.2 WHEN a package name "X" exists in the manifest as a cask but locally as a formula (cross-type presence) THEN the system classifies "X" as ToInstall, because Step 3 checks only `localCasksMap` for manifest casks and the single `seen` map in Step 4 prevents the local formula from being classified as ToRemove — resulting in "X" appearing in ToInstall despite being installed locally under a different type

1.3 WHEN a package name "X" exists in the manifest as a formula but locally as a cask (cross-type presence) THEN the system classifies "X" as ToInstall, because Step 3 checks only `localFormulaeMap` for manifest formulae and misses the local cask with the same name

1.4 WHEN a package name "X" exists in both manifest formulae and local casks, and also in manifest casks and local formulae THEN the system may produce multiple misclassifications for the same name, with "X" appearing in more than one result set

### Expected Behavior (Correct)

2.1 WHEN a package name "X" exists in both local formulae and local casks (but not in the manifest) THEN the system SHALL add "X" to the ToRemove set exactly once per type — once for the formula and once for the cask — or track type-qualified entries so that each (name, type) pair appears in exactly one result set

2.2 WHEN a package name "X" exists in the manifest as a cask but locally as a formula THEN the system SHALL recognize that "X" is present locally (as a formula) and SHALL NOT classify the manifest cask entry as ToInstall; the system SHALL classify it according to the type-aware comparison (e.g., the manifest cask is ToInstall because no local cask exists, and the local formula is ToRemove because no manifest formula exists)

2.3 WHEN a package name "X" exists in the manifest as a formula but locally as a cask THEN the system SHALL recognize the cross-type mismatch and SHALL classify the manifest formula as ToInstall (no local formula exists) and the local cask as ToRemove (no manifest cask exists)

2.4 WHEN a package name "X" exists across multiple type combinations in manifest and local THEN the system SHALL classify each (name, type) pair independently, so that formulae are compared only against local formulae and casks are compared only against local casks

### Unchanged Behavior (Regression Prevention)

3.1 WHEN all package names are unique across manifest formulae, manifest casks, local formulae, and local casks (no cross-type collisions) THEN the system SHALL CONTINUE TO classify every package into exactly one of {ToInstall, ToRemove, ToUpgrade, Unchanged}

3.2 WHEN a manifest formula exists and a local formula with the same name exists THEN the system SHALL CONTINUE TO classify the package as Unchanged (matching versions) or ToUpgrade (different versions)

3.3 WHEN a manifest cask exists and a local cask with the same name exists THEN the system SHALL CONTINUE TO classify the package as Unchanged (matching versions) or ToUpgrade (different versions)

3.4 WHEN a package exists only in the manifest (no local counterpart of any type) THEN the system SHALL CONTINUE TO classify the package as ToInstall

3.5 WHEN a package exists only locally (no manifest counterpart of any type) THEN the system SHALL CONTINUE TO classify the package as ToRemove

3.6 WHEN machine-specific filters (only_on, except_on) exclude a manifest entry THEN the system SHALL CONTINUE TO exclude that entry from the diff computation before classification
