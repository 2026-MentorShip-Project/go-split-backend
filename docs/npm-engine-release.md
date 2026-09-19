# npm split-engine releases

The backend repository publishes `@go-split/engine`. The separate frontend repository pins it as a normal npm dependency. GitHub Actions builds the Go binary; end-user browsers download it from the deployed frontend, not from npm.

## What the npm workflow does

`.github/workflows/npm-engine.yaml` builds and verifies the package, installs the packed tarball into an independent temporary consumer, and uploads that exact tarball as a workflow artifact. This runs without npm credentials.

The release workflow runs only when an `engine-vX.Y.Z` tag is pushed. Branch pushes and manual dispatch do not trigger it. The backend `cd.yaml` excludes engine release tags and guards manual runs on those tags, so engine-only releases skip the GCP deployment job. They do not migrate databases, change infrastructure or deploy Cloud Run. Normal pushes to main retain the existing backend CD behavior and do not publish to npm.

The tag, `packages/split-engine/package.json` version, and `internal/splitengine/engine.go`'s `Version` must match. Only stable X.Y.Z releases are accepted for now. npm publication uses the already-tested tarball; package lifecycle scripts are disabled during the credentialed publish step. An existing npm version is skipped rather than overwritten. Registry errors other than 404 fail the check. Re-running the same tag therefore does not attempt another immutable publication.

PR CI builds, packs and tests the package without publishing. No npm token is available to package tests.

## One-time npm configuration

1. Ensure your npm organization owns `@go-split` and the publisher has permission to publish `@go-split/engine`. If a different scope is needed, update the package name and import examples before the first release.
2. For the first publication, configure the repository Actions secret `NPM_TOKEN` with a valid granular npm token authorized to publish this package (including the automation/2FA configuration required by npm). Do not commit a token. Alternatively, bootstrap the package from an authorized maintainer's machine.
3. Once the package exists, prefer configuring an npm trusted publisher for:
   - GitHub owner: `2026-MentorShip-Project`
   - Repository: `go-split-backend`
   - Workflow filename: `npm-engine.yaml`
   - Environment: leave empty, matching this workflow
   - Allow publishing in the trusted publisher configuration.
4. Remove `NPM_TOKEN` after trusted publishing is configured. The workflow uses GitHub OIDC when no token is supplied. It uses Node 24 and npm 11.5.1 or later. Publishing includes provenance; the repository/package must satisfy npm's provenance requirements.

No npm account, organization, token or trusted-publisher setting is created by this code change. Branch pushes do not publish a package. See [npm trusted publishers](https://docs.npmjs.com/trusted-publishers/) and [npm provenance requirements](https://docs.npmjs.com/generating-provenance-statements/).

## Release procedure

1. Bump the Go engine version and npm manifest together. Wrapper-only releases also need a new matching version.
2. Merge the reviewed change. Deploy the corresponding backend when calculation behavior changes.
3. Create and push the matching tag, for example:

```sh
git tag engine-v1.1.0 <reviewed-commit>
git push origin engine-v1.1.0
```

4. Verify the `publish-engine` job succeeds. The package's npm `latest` tag points to the new stable release.
5. In the frontend repository:

```sh
npm install --save-exact @go-split/engine@1.1.0
```

Commit the frontend manifest/lockfile and redeploy it. npm publication does not automatically update the frontend or backend server. Keep their calculation versions aligned; backend validation remains authoritative.

The import API and bundler/static-asset options are documented in [the package README](../packages/split-engine/README.md). Go's runtime and its redistribution notice are bundled with the WASM; consumers do not install Go. The package declares UNLICENSED rather than assigning a new open-source license to this repository.
