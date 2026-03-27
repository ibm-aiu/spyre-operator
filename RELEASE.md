# Release

> [!NOTE]
> This `RELEASE.md` focuses on the **release policy** and **upcoming release plan**. The detailed how the release process is executed is documented separately in [Developer Guide - Creating a Release](./docs/developer-guide.md#creating-a-release).

This repository follows a hybrid release model:

- Regular releases every three months (quarterly)
- Need-based releases when critical fixes or high-priority features are required

This approach balances predictability for users with flexibility for urgent changes.
<!-- TOC tocDepth:2..3 chapterDepth:2..6 -->

- [Regular (Quarterly) Releases](#regular-quarterly-releases)
    - [Schedule](#schedule)
    - [Scope](#scope)
- [Need-Based (Out-of-Cycle) Releases](#need-based-out-of-cycle-releases)
    - [When They Are Triggered](#when-they-are-triggered)
    - [Characteristics](#characteristics)
- [Release Life Cycle](#release-life-cycle)
- [Upcoming Release Schedule](#upcoming-release-schedule)

<!-- /TOC -->
---

## Regular (Quarterly) Releases

### Schedule

Regular releases are planned on a **three-month cadence**.

| Release | Target Month |
|--------|--------------|
| Q1 Release | March |
| Q2 Release | June |
| Q3 Release | September |
| Q4 Release | December |

> Exact release dates may vary depending on scope, testing, and readiness.

### Scope

Quarterly releases typically include:

- New features
- Enhancements and improvements
- Non-critical bug fixes
- Deprecations (with advance notice)

Each regular release:

- Follows semantic versioning (e.g. `v1.4.0`)
- Is published on the GitHub Releases page

---

## Need-Based (Out-of-Cycle) Releases

Need-based releases may occur **outside the regular quarterly schedule**.

### When They Are Triggered

These releases are initiated when one or more of the following apply:

- Critical bug fixes
- Security vulnerabilities
- Blocking issues affecting users or systems
- High-priority operational or customer needs

### Characteristics

- Smaller and more focused scope
- Released as soon as stability and quality criteria are met
- Typically issued as patch versions (e.g. `v1.4.1`)
- Minor versions may be used if limited functionality is added

---

## Release Life Cycle

Each release progresses through the following life cycle stages:

1. **Feature Freeze**
   *(2 weeks after previous release cut)*
   - All target features must be marked with the release milestone and labeled with `enhancement` prior to this freeze due
   - No new features or enhancements are accepted
   - Release scope is finalized

2. **Code Freeze**
   *(1 weeks before release)*
   - No code changes except critical bug fixes
   - All changes require explicit approval
   - Focus on stabilization and validation

3. **Release**
   - Version is tagged and released
   - Artifacts are built and published
   - Release is made available on the GitHub Releases page

4. **Post-Release**
   - Monitor stability and collect feedback
   - Address critical issues through need-based releases
   - Close milestones and begin planning for the next release cycle

5. **End of Life (EOL)**
   *(6 months after release)*
   - The release is no longer supported or maintained
   - Security patches and bug fixes are not provided
   - Users are encouraged to upgrade to a supported release

---

## Upcoming Release Schedule

The project follows a **regular quarterly release cadence**, with additional **need-based releases** as required.

| Release | Planned Release | Feature Freeze | Code Freeze | Notes |
|---------|-----------------|----------------|-------------|-------|
| v1.2.0  |   2026-03-31    |   2026-02-06   |  2026-03-24 | Regular feature and improvement release. Check [this milestone](https://github.ibm.com/ai-chip-toolchain/aiu-operator/milestone/21) for the planned enhancements.|
