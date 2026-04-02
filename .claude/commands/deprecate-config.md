<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

Deprecate a config key using the struct-tag deprecation framework.

The user provides: the old key name, the new key name, and whether the
value format changes. You handle the rest.

## 1. Add the `deprecated` tag

On the **new** field's struct tag in `internal/config/config.go`, add:

```
deprecated:"old_key"
```

If the value format changes (e.g., integer days to duration string), also
add a `deprecated_transform` tag naming a registered hint in
`internal/config/deprecated.go`:

```
deprecated:"old_key" deprecated_transform:"hint_name"
```

Register the hint string in `transformHints` if it doesn't exist yet.

## 2. Update ExampleTOML

In `ExampleTOML()` in `config.go`, replace the old key name with the new
one in all commented examples.

## 3. Update field doc comments

Rename the Go doc comment on the field to match the new name.

## 4. Regenerate deprecation data

```
go generate ./internal/config/
```

Verify `docs/data/deprecations.json` includes the new entry.

## 5. Update Hugo docs

In `docs/content/docs/reference/configuration.md`:

- Replace the old key name with the new one in the config table row
- Replace the old `{{< env >}}` var name with the new one
- Add `{{< replaces "section.new_key" >}}` shortcode to the row

## 6. Update references

Search for the old field name across the codebase:

```
rg 'OldFieldName|old_key' --type go
```

Update all call sites: struct literals, method calls, CLI wiring.

## 7. Update tests

- Rename test functions and TOML fixtures from old key to new key
- Add a test that using the old key in TOML returns the expected hard error
- Add a test that using the old env var returns the expected hard error
- Update `TestEnvVars` expected map entries

## 8. Verify

```
go test -shuffle=on ./internal/config/
go build ./...
```

## 9. Commit

Use `/commit`. The type is `refactor(config):` for renames.
