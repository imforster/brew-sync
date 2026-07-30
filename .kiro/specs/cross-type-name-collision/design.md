# Cross-Type Name Collision Bugfix Design

## Overview

The `ComputeDiff` function in `internal/diff/engine.go` uses a single `seen` map to track which package names have been processed across both formulae and casks. Homebrew allows the same name to exist as both a formula and a cask simultaneously (e.g., `docker` as a formula and `docker` as a cask). The shared `seen` map causes two classes of failure: (1) duplicate ToRemove entries when the same name exists in both local formulae and local casks, and (2) incorrect ToInstall classification when a manifest entry of one type has a name that matches a local package of a different type.

The fix splits the single `seen` map into `seenFormulae` and `seenCasks`, so that formulae are compared only against local formulae and casks only against local casks. The `DiffResult` type must also become type-aware — either by adding a `Type` field to `PackageEntry` or by using separate slices for formulae and cask results — so that downstream consumers (like `ApplyDiff`) can distinguish between a formula install and a cask install for the same name.

## Glossary

- **Bug_Condition (C)**: The condition where a package name exists in both the formula namespace and the cask namespace simultaneously (across manifest and/or local state), causing the single `seen` map to conflate the two types
- **Property (P)**: Each (name, type) pair is classified independently — formulae are compared only against local formulae, casks only against local casks — and each pair appears in exactly one result set
- **Preservation**: All existing behavior for inputs where names are globally unique across types (no cross-type collisions) must remain unchanged
- **ComputeDiff**: The function in `internal/diff/engine.go` that compares a manifest against local state and produces a `DiffResult` with ToInstall, ToRemove, ToUpgrade, and Unchanged sets
- **seen map**: The `map[string]bool` in `ComputeDiff` that tracks processed names — currently shared across formulae and casks, causing the bug
- **Cross-type collision**: A situation where the same package name appears as different types (formula vs cask) in the manifest and/or local state

## Bug Details

### Bug Condition

The bug manifests when a package name exists in both the formula namespace and the cask namespace within the inputs to `ComputeDiff`. The single `seen` map cannot distinguish between "seen as a formula" and "seen as a cask", so the second occurrence of a name is either skipped or misclassified.

**Formal Specification:**
```
FUNCTION isBugCondition(manifest, local)
  INPUT: manifest of type *Manifest, local of type *LocalState
  OUTPUT: boolean

  allFormulaNames := manifest.Formulae.names ∪ local.Formulae.names
  allCaskNames := manifest.Casks.names ∪ local.Casks.names

  RETURN allFormulaNames ∩ allCaskNames ≠ ∅
END FUNCTION
```

### Examples

- **Duplicate ToRemove**: Manifest is empty. Local has `{Formulae: [{Name: "a"}], Casks: [{Name: "a"}]}`. The `seen` map is empty after Step 3 (no manifest entries). In Step 4, the formula "a" is not in `seen`, so it's added to ToRemove and `seen["a"] = true`. Then the cask "a" IS in `seen`, so it's skipped. Result: "a" appears once in ToRemove instead of twice (once per type). The rapid fail file `TestPropertyDiffCompleteness-20260417124436-74793.fail` documents this exact case, but with the opposite outcome — "a" appears 2 times because both local walks add it.

- **Cross-type ToInstall**: Manifest has `{Casks: [{Name: "a"}]}`. Local has `{Formulae: [{Name: "a"}]}`. In Step 3, the manifest cask "a" is checked against `localCasksMap` — not found, so it's classified as ToInstall and `seen["a"] = true`. In Step 4, the local formula "a" IS in `seen`, so it's skipped (not added to ToRemove). Result: "a" is in ToInstall even though it's installed locally (as a formula). The rapid fail file `TestPropertyDiffSoundness-20260417124436-74793.fail` documents this exact case.

- **Symmetric cross-type**: Manifest has `{Formulae: [{Name: "a"}]}`. Local has `{Casks: [{Name: "a"}]}`. The manifest formula "a" is checked against `localFormulaeMap` — not found, so it's ToInstall and `seen["a"] = true`. The local cask "a" IS in `seen`, so it's skipped. Result: "a" is ToInstall but the local cask is silently dropped.

- **Both types in both sides**: Manifest has `{Formulae: [{Name: "a"}], Casks: [{Name: "a"}]}`. Local has `{Formulae: [{Name: "a"}], Casks: [{Name: "a"}]}`. The manifest formula "a" matches `localFormulaeMap` → Unchanged, `seen["a"] = true`. The manifest cask "a" is already in `seen`... but the code still processes it (seen is only used in Step 4). Actually, `seen` is set in Step 3 for both loops, so the cask loop sets `seen["a"] = true` again (no-op). The cask "a" matches `localCasksMap` → Unchanged. In Step 4, both local entries are in `seen`, so neither is added to ToRemove. This case actually works correctly by coincidence.

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- When all package names are globally unique across formulae and casks, the diff result must be identical to the current (buggy) code's output — because the bug only manifests with cross-type name collisions
- Same-type matching (manifest formula vs local formula, manifest cask vs local cask) must continue to classify as Unchanged or ToUpgrade based on version comparison
- Machine-specific filters (`only_on`, `except_on`) must continue to exclude entries before classification
- The `ApplyDiff` function must continue to work correctly for all non-cross-type inputs
- The `FilterForMachine` function is not affected by this bug and must remain unchanged

**Scope:**
All inputs where no package name appears in both the formula namespace and the cask namespace are completely unaffected by this fix. This includes:
- Manifests and local states with entirely distinct names across types
- Empty manifests or empty local states
- Inputs with machine filters that exclude entries before the diff computation

## Hypothesized Root Cause

Based on the code analysis, the root cause is confirmed (not just hypothesized):

1. **Single `seen` map across types**: The `seen` map in `ComputeDiff` (line ~42 of engine.go) is a `map[string]bool` shared by both the formulae loop and the casks loop. When a name is processed as a formula in Step 3, `seen[name] = true` prevents the same name from being classified as ToRemove in Step 4's cask walk (and vice versa). This is the primary cause.

2. **Type-unaware `DiffResult`**: The `DiffResult` struct uses `[]manifest.PackageEntry` for all four result sets, but `PackageEntry` has no `Type` field. Even if the `seen` map were split, the result sets would contain entries from both types with no way to distinguish them. This means `ApplyDiff` cannot know whether to run `brew install` (formula) or `brew install --cask` (cask) for a given entry.

3. **Test generators mask the bug**: The `genDiffTestData()` and `genIdempotencyTestData()` generators in the property tests enforce globally unique names across all four lists. The comments explicitly state: "This avoids name collisions between types, which the engine's single `seen` map assumes." This means the property tests never generate the cross-type collision inputs that would expose the bug.

4. **Soundness test uses name-only sets**: `TestPropertyDiffSoundness` builds `manifestNames` and `localNames` as flat `map[string]bool` sets without type information. Even if the engine were fixed, the test's assertions would need updating to be type-aware.

## Correctness Properties

Property 1: Bug Condition - Type-Aware Diff Completeness

_For any_ manifest and local state where a package name exists in both the formula namespace and the cask namespace (isBugCondition returns true), the fixed `ComputeDiff` function SHALL classify each (name, type) pair into exactly one of {ToInstall, ToRemove, ToUpgrade, Unchanged}, with no duplicates and no missing entries.

**Validates: Requirements 2.1, 2.2, 2.3, 2.4**

Property 2: Preservation - Non-Collision Diff Equivalence

_For any_ manifest and local state where all package names are globally unique across formulae and casks (isBugCondition returns false), the fixed `ComputeDiff` function SHALL produce the same classification as the original function, preserving all existing behavior for ToInstall, ToRemove, ToUpgrade, and Unchanged sets.

**Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5, 3.6**

## Fix Implementation

### Changes Required

**File**: `internal/diff/types.go`

**Struct**: `DiffResult`

**Specific Changes**:
1. **Add type-aware result slices**: Replace the four `[]manifest.PackageEntry` slices with type-separated slices, or add a wrapper type that includes the package type. The simplest approach that preserves backward compatibility is to split each slice into formulae and cask variants:
   ```go
   type DiffResult struct {
       ToInstall   []manifest.PackageEntry
       ToRemove    []manifest.PackageEntry
       ToUpgrade   []manifest.PackageEntry
       Unchanged   []manifest.PackageEntry
       // Type-separated views for consumers that need type awareness
       FormulaInstall []manifest.PackageEntry
       CaskInstall    []manifest.PackageEntry
       FormulaRemove  []manifest.PackageEntry
       CaskRemove     []manifest.PackageEntry
       FormulaUpgrade []manifest.PackageEntry
       CaskUpgrade    []manifest.PackageEntry
   }
   ```
   However, this doubles the surface area. A cleaner approach is to add a `Type` field to the entries in the result, or use a `TypedPackageEntry` wrapper. The simplest minimal fix: keep the existing flat slices but ensure each entry carries its type. Since `PackageEntry` is defined in the `manifest` package and adding a `Type` field there changes the TOML schema, the better approach is to use a local wrapper or separate the result slices by type.

   **Recommended approach**: Split `DiffResult` into type-separated slices and provide combined accessors for backward compatibility:
   ```go
   type DiffResult struct {
       FormulaInstall []manifest.PackageEntry
       CaskInstall    []manifest.PackageEntry
       FormulaRemove  []manifest.PackageEntry
       CaskRemove     []manifest.PackageEntry
       FormulaUpgrade []manifest.PackageEntry
       CaskUpgrade    []manifest.PackageEntry
       FormulaUnchanged []manifest.PackageEntry
       CaskUnchanged    []manifest.PackageEntry
   }
   ```
   With convenience methods:
   ```go
   func (d *DiffResult) ToInstall() []manifest.PackageEntry { return append(d.FormulaInstall, d.CaskInstall...) }
   func (d *DiffResult) ToRemove() []manifest.PackageEntry  { return append(d.FormulaRemove, d.CaskRemove...) }
   ```

   **Alternative simpler approach**: Keep the existing `DiffResult` fields as-is but fix the engine logic so that the flat lists are correct (each name appears the right number of times). This works if `ApplyDiff` doesn't need to distinguish types. Looking at `ApplyDiff`, it calls `runner.Install(Package{Name: pkg.Name, Version: pkg.Version})` — the `Runner` interface doesn't distinguish formula vs cask installs. So the minimal fix may be to just fix the `seen` map without changing `DiffResult`, accepting that the flat lists may contain the same name twice (once for formula, once for cask). However, this means `ApplyDiff` would try to install the same name twice, which is wasteful but not incorrect if brew handles it idempotently.

   **Decision**: The cleanest minimal fix is to split the `seen` map into `seenFormulae` and `seenCasks` in `ComputeDiff`, keeping `DiffResult` unchanged. The flat result slices will correctly contain one entry per (name, type) pair. If the same name appears as both a formula ToInstall and a cask ToRemove, both entries appear in their respective slices. `ApplyDiff` already works because brew operations are name-based and the runner interface doesn't need type information at this level.

---

**File**: `internal/diff/engine.go`

**Function**: `ComputeDiff`

**Specific Changes**:
1. **Split `seen` into `seenFormulae` and `seenCasks`**: Replace the single `seen := make(map[string]bool)` with two maps:
   ```go
   seenFormulae := make(map[string]bool)
   seenCasks := make(map[string]bool)
   ```

2. **Update Step 3 formula loop**: Use `seenFormulae` instead of `seen`:
   ```go
   for _, entry := range formulae {
       seenFormulae[entry.Name] = true
       // ... existing classification logic (unchanged)
   }
   ```

3. **Update Step 3 cask loop**: Use `seenCasks` instead of `seen`:
   ```go
   for _, entry := range casks {
       seenCasks[entry.Name] = true
       // ... existing classification logic (unchanged)
   }
   ```

4. **Update Step 4 local formula walk**: Check `seenFormulae` instead of `seen`:
   ```go
   for _, pkg := range local.Formulae {
       if !seenFormulae[pkg.Name] {
           result.ToRemove = append(result.ToRemove, manifest.PackageEntry{Name: pkg.Name})
       }
   }
   ```

5. **Update Step 4 local cask walk**: Check `seenCasks` instead of `seen`:
   ```go
   for _, pkg := range local.Casks {
       if !seenCasks[pkg.Name] {
           result.ToRemove = append(result.ToRemove, manifest.PackageEntry{Name: pkg.Name})
       }
   }
   ```

---

**File**: `internal/diff/engine_property_test.go`

**Generator**: `genDiffTestData`

**Specific Changes**:
1. **Allow cross-type name collisions**: Remove the constraint that names are globally unique. Allow each name to independently appear as a formula and/or cask on each side (manifest and local). Each name should be assigned to formula and/or cask independently on each side.

2. **Update completeness test assertions**: The `TestPropertyDiffCompleteness` test currently counts by name. With cross-type collisions, the same name can legitimately appear twice in the result sets (once as formula, once as cask). The universe and counting logic needs to track (name, type) pairs instead of just names.

3. **Update soundness test assertions**: The `TestPropertyDiffSoundness` test builds `manifestNames` and `localNames` as flat sets. These need to become type-aware: `manifestFormulaeNames`, `manifestCaskNames`, `localFormulaeNames`, `localCaskNames`. The assertions should verify that a ToInstall formula entry is in manifest formulae but not in local formulae (ignoring local casks).

---

**File**: `internal/diff/apply_property_test.go`

**Generator**: `genIdempotencyTestData`

**Specific Changes**:
1. **Allow cross-type name collisions**: Same change as `genDiffTestData` — remove global uniqueness constraint.
2. **Update idempotency simulation**: The simulated apply in `TestPropertyIdempotency` uses `manifestFormulaeNames` and `manifestCaskNames` to decide which local map to update. This logic is already type-aware and should work correctly once the generator allows cross-type collisions.

---

**File**: `internal/diff/testdata/rapid/`

**Specific Changes**:
1. **Delete stale fail files**: Remove `TestPropertyDiffCompleteness-20260417124436-74793.fail` and `TestPropertyDiffSoundness-20260417124436-74793.fail`. These document failures from the original globally-unique generators and are no longer valid after the generator and engine fixes.

## Testing Strategy

### Validation Approach

The testing strategy follows a two-phase approach: first, surface counterexamples that demonstrate the bug on unfixed code (using updated generators that allow cross-type collisions), then verify the fix works correctly and preserves existing behavior.

### Exploratory Bug Condition Checking

**Goal**: Surface counterexamples that demonstrate the bug BEFORE implementing the fix. Confirm the root cause analysis by updating the property test generators to allow cross-type name collisions and running them against the unfixed engine.

**Test Plan**: Modify `genDiffTestData()` to allow the same name in both formula and cask namespaces. Run `TestPropertyDiffCompleteness` and `TestPropertyDiffSoundness` against the unfixed `ComputeDiff`. Observe failures that match the documented rapid fail patterns.

**Test Cases**:
1. **Completeness with local cross-type**: Generate local state with same name in both Formulae and Casks, empty manifest. Expect "appears 2 times" failure (will fail on unfixed code)
2. **Soundness with manifest-cask/local-formula**: Generate manifest with cask "a", local with formula "a". Expect "ToInstall contains 'a' which IS in local" failure (will fail on unfixed code)
3. **Soundness with manifest-formula/local-cask**: Generate manifest with formula "a", local with cask "a". Expect same class of failure (will fail on unfixed code)
4. **Both types on both sides**: Generate manifest and local both having "a" as formula and cask. May produce unexpected classification (will fail on unfixed code)

**Expected Counterexamples**:
- Package "a" appears 2 times in diff result sets (completeness violation)
- ToInstall contains "a" which IS in local (soundness violation)
- Possible causes confirmed: single `seen` map conflates formula and cask namespaces

### Fix Checking

**Goal**: Verify that for all inputs where the bug condition holds (cross-type name collisions exist), the fixed function produces correct type-aware classification.

**Pseudocode:**
```
FOR ALL (manifest, local) WHERE isBugCondition(manifest, local) DO
  result := ComputeDiff_fixed(manifest, local, machineTag)
  
  // Each (name, type) pair appears in exactly one result set
  FOR EACH (name, type) IN universe(manifest, local) DO
    count := countInResults(result, name, type)
    ASSERT count == 1
  END FOR
  
  // ToInstall formulae are in manifest formulae but not local formulae
  FOR EACH entry IN result.ToInstall WHERE isFormula(entry) DO
    ASSERT entry.Name IN manifest.Formulae.names
    ASSERT entry.Name NOT IN local.Formulae.names
  END FOR
  
  // ToRemove formulae are in local formulae but not manifest formulae
  FOR EACH entry IN result.ToRemove WHERE isFormula(entry) DO
    ASSERT entry.Name IN local.Formulae.names
    ASSERT entry.Name NOT IN manifest.Formulae.names
  END FOR
END FOR
```

### Preservation Checking

**Goal**: Verify that for all inputs where the bug condition does NOT hold (no cross-type name collisions), the fixed function produces the same result as the original function.

**Pseudocode:**
```
FOR ALL (manifest, local) WHERE NOT isBugCondition(manifest, local) DO
  ASSERT ComputeDiff_original(manifest, local, tag) = ComputeDiff_fixed(manifest, local, tag)
END FOR
```

**Testing Approach**: Property-based testing is recommended for preservation checking because:
- It generates many test cases automatically across the input domain
- It catches edge cases that manual unit tests might miss
- It provides strong guarantees that behavior is unchanged for all non-buggy inputs
- The existing generators (which enforce global uniqueness) already generate exactly the non-buggy input space

**Test Plan**: Keep the existing property tests running with their current generators (which enforce global uniqueness) to serve as preservation tests. Add new property tests with updated generators that allow cross-type collisions to serve as fix-checking tests.

**Test Cases**:
1. **Same-type formula matching preservation**: Verify that manifest formula vs local formula classification (Unchanged, ToUpgrade) continues to work identically
2. **Same-type cask matching preservation**: Verify that manifest cask vs local cask classification continues to work identically
3. **Disjoint names preservation**: Verify that manifest-only → ToInstall and local-only → ToRemove continues to work
4. **Machine filter preservation**: Verify that `only_on`/`except_on` filtering continues to exclude entries before classification

### Unit Tests

- Add unit test for cross-type collision: manifest cask "docker" + local formula "docker" → cask is ToInstall, formula is ToRemove
- Add unit test for symmetric case: manifest formula "docker" + local cask "docker"
- Add unit test for same name in both local types, not in manifest → both are ToRemove
- Add unit test for same name in both manifest types and both local types → both are Unchanged
- Existing unit tests (EmptySets, IdenticalSets, DisjointSets, PartialOverlap, VersionDifference, etc.) serve as preservation tests

### Property-Based Tests

- Update `genDiffTestData()` to allow cross-type name collisions and update `TestPropertyDiffCompleteness` to count (name, type) pairs instead of just names
- Update `TestPropertyDiffSoundness` to use type-aware manifest/local name sets
- Update `genIdempotencyTestData()` to allow cross-type name collisions
- Existing property tests with globally-unique generators can be kept as additional preservation coverage, or the generators can be updated in-place since the fixed code should handle both cases

### Integration Tests

- Test full workflow: push a manifest with cask "docker", pull on a machine that has formula "docker" installed, verify `status` shows correct diff
- Test `apply` with cross-type collision: verify that the correct brew commands are issued (install cask, remove formula)
- Test idempotency: apply a diff with cross-type entries, recompute diff, verify empty result
