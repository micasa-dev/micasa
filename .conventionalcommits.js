// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0



const { readFileSync } = require("node:fs");

const releaseConfig = JSON.parse(readFileSync("./.releaserc.json", "utf8"));

const types = releaseConfig.plugins
  .filter((p) => Array.isArray(p) && p[0] === "@semantic-release/release-notes-generator")
  .map((p) => p[1].presetConfig.types)[0];

module.exports = {
  options: {
    preset: {
      name: "conventionalcommits",
      types,
    },
  },
  writerOpts: {
    finalizeContext(ctx) {
      ctx.linkCompare = false;
      return ctx;
    },
  },
};
