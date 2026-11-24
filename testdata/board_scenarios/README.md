# Board registry scenarios

These synthetic JSON fixtures are loaded by `internal/company/TestRegistryFixtureCorpus` and run through the PostgreSQL registry, policy-gated discovery, and provider validation paths.

- `no_board.json` confirms an empty observation stays empty and does not imply company closure.
- `multiple_boards.json` confirms Greenhouse and Ashby candidates are both retained and repeat discovery is idempotent.
- `migrated_ats.json` keeps the historical Greenhouse board inactive while discovering and validating the current Ashby board.
- `inaccessible_board.json` confirms a provider 403 remains visible as `blocked_by_policy`.

All provider responses and source policies are test-local. No public endpoint is contacted.
