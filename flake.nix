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
