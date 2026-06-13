# Nix Flake Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a minimal Nix flake that builds and exposes the `xapi-usecase` CLI for installation and `nix run`.

**Architecture:** Follow the `driftline` flake structure directly: one `buildGoModule` package per supported system, with `packages.default` and `apps.default` pointing at the same CLI. Keep the flake installation-focused and avoid development shells, checks, formatters, or CI changes.

**Tech Stack:** Nix flakes, `nixpkgs` `buildGoModule`, Go 1.26.3, standard library only.

---

Repository instruction: do not create commits unless the user explicitly asks. This plan uses verification checkpoints instead of commit steps.

## File Structure

- Create `flake.nix`: define nixpkgs input, supported systems, `xapi-usecase` package, default package, `xapi-usecase` app, and default app.

## Task 1: Add Minimal Flake

**Files:**

- Create: `flake.nix`

- [ ] **Step 1: Add `flake.nix`**

Create `flake.nix` with this content:

```nix
{
  description = "xapi-usecase CLI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forEachSystem = f:
        nixpkgs.lib.genAttrs systems (system:
          f system (import nixpkgs { inherit system; })
        );
    in
    {
      packages = forEachSystem (_system: pkgs:
        let
          xapi-usecase = pkgs.buildGoModule {
            pname = "xapi-usecase";
            version = "0.0.0";

            src = pkgs.lib.cleanSourceWith {
              src = ./.;
              filter = path: _type:
                let
                  rel = pkgs.lib.removePrefix "${toString ./.}/" (toString path);
                in
                rel == "go.mod"
                || rel == "cmd"
                || rel == "internal"
                || pkgs.lib.hasPrefix "cmd/" rel
                || pkgs.lib.hasPrefix "internal/" rel;
            };
            subPackages = [ "cmd/xapi-usecase" ];

            vendorHash = null;

            ldflags = [
              "-s"
              "-w"
            ];

            meta = {
              description = "CLI for X API v2 use cases";
              homepage = "https://github.com/y-writings/xapi-usecase";
              license = pkgs.lib.licenses.mit;
              mainProgram = "xapi-usecase";
            };
          };
        in
        {
          inherit xapi-usecase;
          default = xapi-usecase;
        });

      apps = forEachSystem (system: _pkgs:
        let
          xapi-usecase = {
            type = "app";
            program = "${self.packages.${system}.xapi-usecase}/bin/xapi-usecase";
          };
        in
        {
          inherit xapi-usecase;
          default = xapi-usecase;
        });
    };
}
```

- [ ] **Step 2: Run flake verification**

Run:

```sh
nix flake check
```

Expected: success.

- [ ] **Step 3: Build the package**

Run:

```sh
nix build .#xapi-usecase
```

Expected: success and `result/bin/xapi-usecase` exists.

- [ ] **Step 4: Smoke-test the binary**

Run:

```sh
./result/bin/xapi-usecase --help
```

Expected: exit code `0` and usage text for `xapi-usecase`.

## Self-Review

- The plan covers the approved minimal install-only flake.
- There are no placeholders or compatibility shims.
- The verification commands exercise flake evaluation, package build, and executable startup.
