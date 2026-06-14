# Nix Flake Install Design

## Context

`xapi-usecase` is a pre-release Go CLI. It currently builds with Go 1.26.3 and only uses the Go standard library. The executable entry point is `cmd/xapi-usecase`, and the repository does not have existing Nix files.

The requested model is `https://github.com/y-writings/driftline`: a minimal flake that exposes installable packages and apps for common Linux and Darwin systems.

## Goals

- Add a minimal `flake.nix` for installing and running `xapi-usecase` with Nix.
- Match the `driftline` shape: `packages.${system}.xapi-usecase`, `packages.${system}.default`, `apps.${system}.xapi-usecase`, and `apps.${system}.default`.
- Support `x86_64-linux`, `aarch64-linux`, `x86_64-darwin`, and `aarch64-darwin`.
- Build `cmd/xapi-usecase` with `pkgs.buildGoModule`.
- Keep the flake focused on installation only.

## Non-Goals

- Do not add `devShell`, `checks`, `formatter`, CI changes, or README changes.
- Do not add compatibility layers or alternative package names.
- Do not introduce external dependencies.

## Design

`flake.nix` imports `nixpkgs` from `github:NixOS/nixpkgs/nixos-unstable`, defines the same four-system matrix as `driftline`, and uses a small `forEachSystem` helper.

The package uses `pkgs.buildGoModule` with:

- `pname = "xapi-usecase"`
- `version = "0.0.0"`
- `src = pkgs.lib.cleanSourceWith` limited to `go.mod`, `cmd/`, and `internal/`
- `subPackages = [ "cmd/xapi-usecase" ]`
- `vendorHash = null` because the module has no external Go dependencies
- stripped `ldflags`
- MIT metadata and `mainProgram = "xapi-usecase"`

The app output points at `${self.packages.${system}.xapi-usecase}/bin/xapi-usecase`.

## Verification

- Run `nix flake check`.
- Run `nix build .#xapi-usecase`.
- Run `./result/bin/xapi-usecase --help` and confirm the CLI prints usage.
