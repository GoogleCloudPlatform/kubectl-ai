# How to Contribute

We would love to accept your patches and contributions to this project.

## Before you begin

### Sign our Contributor License Agreement

Contributions to this project must be accompanied by a
[Contributor License Agreement](https://cla.developers.google.com/about) (CLA).
You (or your employer) retain the copyright to your contribution; this simply
gives us permission to use and redistribute your contributions as part of the
project.

If you or your current employer have already signed the Google CLA (even if it
was for a different project), you probably don't need to do it again.

Visit <https://cla.developers.google.com/> to see your current agreements or to
sign a new one.

### Review our Community Guidelines

This project follows [Google's Open Source Community
Guidelines](https://opensource.google/conduct/).

## Contribution process

### Code Reviews

All submissions, including submissions by project members, require review. We
use [GitHub pull requests](https://docs.github.com/articles/about-pull-requests)
for this purpose.

## Understand the repo

An AI-generated overview of the system architecture for this repository is
available [here](https://deepwiki.com/GoogleCloudPlatform/kubectl-ai/). This can
provide an interactive way to explore the codebase.

Quick notes about the various directories:
- Source code for `kubectl-ai` CLI lives under `cmd/` and `pkg/` directories.
- gollm directory is an independent Go module that implements LLM clients for
different LLM providers.
- `modelserving` directory contains utilities and configuration to build and run
open source AI models locally or in a kubernetes cluster.
- `kubectl-utils` is an independent Go package/binary to help with the benchmarks tasks
that evaluates various conditions involving properties of kubernetes resources.
- User guides/design docs/proposals live under `docs` directory.
- `dev` directory scripts for project related tasks (adhoc/CI).
