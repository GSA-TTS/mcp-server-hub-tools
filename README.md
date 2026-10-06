# Tools

GSA fork of the Obot tools repository. It includes the login.gov authentication
provider used by the cloud.gov MCP gateway.

## Login.gov Providers Image

The `public-providers` Docker target preserves Obot's pinned upstream public
providers and overlays a separate GSA registry containing login.gov:

```text
/obot-providers/gsa-tools/
├── auth-providers/login.gov-auth-provider.yaml
├── auth-providers-common/templates/
└── bin/
    ├── login.gov-auth-provider
    └── login.gov-auth-provider.bin
```

The `Build GSA providers image` workflow builds and verifies this image for
`linux/amd64`, publishes immutable `sha-<commit>` tags to
`ghcr.io/gsa-tts/mcp-server-hub-tools/providers`, and signs the image. The GHCR
package must be public before cloud.gov can build or run an Obot image from it.

Provider configuration is stored through Obot's Auth Providers UI/API as an
encrypted provider credential. Do not place the login.gov private key in this
repository, the Cloud Foundry manifest, or a container image. Obot generates the
cookie secret when the provider is configured.

Local image verification:

```bash
docker buildx build \
  --platform linux/amd64 \
  --target public-providers \
  --load \
  -t local/logingov-providers:verify .
bash ./scripts/verify-logingov-provider-image.sh local/logingov-providers:verify
```

Then pass the published providers image digest to the Obot
`cloudgov-image.yml` workflow. Do not use a mutable providers tag for an Obot
release build.

## Issues
Want to open an issue? Head over to the [Obot repo](https://github.com/obot-platform/obot/issues). Tool related issues will have the `tools` label. 
