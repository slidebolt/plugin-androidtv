# AI Agent Task Instructions

You are assigned to align this plugin with the Slidebolt SDK architecture. 

## Process
1. **Read Tasks**: Read each file in this folder starting with `001-`.
2. **Identify Proof**: Before writing any implementation code, identify the integration or unit tests required to prove the current behavior and verify the new behavior.
3. **Test First**: Write the test first. Run it to see it fail (or pass if it's a regression test).
4. **Implement**: Modify the code to implement the feature.
5. **Verify**: Run local tests (`make test-local-one PKG=...` in the root `work/test` dir) to ensure functionality is preserved.
6. **Gitflow**:
   - Create a feature branch: `feature/task-name`.
   - Commit logically.
   - Push and wait for CI.
7. **Status Update**:
   - Move the task to `REVIEW` by renaming the file or updating its status header.
   - If a task is marked `REJECTED`, re-read the requirements and feedback carefully.

## SDK Architectural Principles
- **Lazy Discovery**: Do not run background loops to discover devices. Perform discovery logic within `OnDevicesList`.
- **No Shadow Registries**: Do not maintain local maps of devices or entities (e.g. `map[string]*Device`). Rely on the `Runner`'s persistence and the `current` slice passed into the handlers.
- **Statelessness**: The plugin should act as a lightweight translator between the provider API and the Slidebolt SDK. Use `RawStore` for protocol-specific metadata (like IP addresses or MAC mappings) that isn't part of the core `Device` type.
