# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

{
  lib,
  buildGoModule,
  gitignoreSource,
}:
let
  pname = "micasa";
  version = builtins.replaceStrings [ "\n" "\r" ] [ "" "" ] (builtins.readFile ../VERSION);
in
buildGoModule {
  inherit pname;
  inherit version;
  src = gitignoreSource ../.;
  subPackages = [ "cmd/micasa" ];
  vendorHash = "sha256-r8zgXVRss4U3EA2iSs9YqLlNlWeANJXIKP8s1O75Wmw=";
  env.CGO_ENABLED = 0;
  preCheck = ''
    export HOME="$(mktemp -d)"
  '';
  ldflags = [
    "-X main.version=${version}"
  ];
  meta = {
    description = "A modal TUI for tracking home projects, maintenance schedules, appliances, and vendor quotes.";
    homepage = "https://micasa.dev";
    changelog = "https://github.com/micasa-dev/micasa/releases/tag/v${version}";
    license = lib.licenses.asl20;
    mainProgram = pname;
  };
}
