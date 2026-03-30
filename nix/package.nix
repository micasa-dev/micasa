# Copyright 2026 Phillip Cloud
# Licensed under the Apache License, Version 2.0

{
  buildGoModule,
  version,
  src,
}:
buildGoModule {
  pname = "micasa";
  inherit version src;
  subPackages = [ "cmd/micasa" ];
  vendorHash = "sha256-r8zgXVRss4U3EA2iSs9YqLlNlWeANJXIKP8s1O75Wmw=";
  env.CGO_ENABLED = 0;
  preCheck = ''
    export HOME="$(mktemp -d)"
  '';
  ldflags = [
    "-X main.version=${version}"
  ];
  meta.mainProgram = "micasa";
}
