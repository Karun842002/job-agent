# Board validation

The validator supports Greenhouse and Ashby public job-board endpoints. Register a reviewed discovery policy for the provider hostname before validating. A missing or denied policy blocks the request. The API endpoint is `POST /api/v1/boards/{id}/validate` and requires the configured bearer token.

Greenhouse board metadata confirms the board is reachable. It does not report listing counts. Ashby responses distinguish listed, unlisted and unknown listings. These counts are observations from the returned response and do not establish complete company coverage. Unavailable, malformed and rate-limited responses leave the board degraded. Permission denials and provider access blocks leave it policy-blocked.

The provider API references are the [Greenhouse Job Board API](https://docs.greenhouse.io/job-board.html) and the [Ashby Job Postings API](https://developers.ashbyhq.com/docs/public-job-posting-api). Provider documentation does not replace source-policy review.

The live integration check is opt-in. ADR-0005 records the owner's temporary permission for the providers' own `greenhouse` and `ashby` boards. The test creates and removes its own policy entries and fixture records, and refuses to replace existing source policies. Run it only with a disposable `TEST_DATABASE_URL` and that specific permission.
