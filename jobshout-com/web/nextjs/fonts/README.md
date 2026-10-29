# Vendored latin fonts

Self-hosted so `next build` inside Docker/CI does not call fonts.googleapis.com
(that fetch is flaky and has failed marketplace image publishes).

Source: [@fontsource](https://fontsource.org/) latin cuts for Unbounded, Poppins,
and JetBrains Mono.
