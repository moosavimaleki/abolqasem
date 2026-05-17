# Generated Web Assets

This directory is generated from `../web-react`.

Do not edit files in this directory directly. Frontend source code lives in:

- `../web-react/src`
- `../web-react/public`
- `../web-react/package.json`

Run this from the repository root to regenerate the embedded web assets:

```sh
sh scripts/prepare-web-assets.sh
```

The Go binary embeds the generated files from this directory at build time.
