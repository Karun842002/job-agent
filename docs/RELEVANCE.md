# Deterministic relevance intake

The initial relevance reader uses the application PostgreSQL evidence database
intended for resume curation. `career_evidence` stores versioned candidate facts
with a source reference and verification status. Only the newest version of
each fact is considered, and it must be verified. The initial structured kinds
are experience, preferred location, target title and skill; they are not the
complete resume evidence model planned for Phase 4.

Source reliability is an explicit `source_reliability` assessment. It is
independent of the source policy that permits discovery: permission alone does
not make a source reliable. A missing assessment is unknown.

The current evaluator compares normalized case-insensitive whole values for
location, title and skills. It does not infer unknown values or synonyms. A
proposal requires a location match, verified career evidence, a reliable
source, and a title or skills match. It checks source permission and the current
`tier_proposal` mode in the transaction that creates the prompt. OFF creates
no prompt; AUDIT writes an isolated preview. Company and role identity
constraints prevent duplicate prompts.

The evidence schema and evaluation path currently have synthetic fixture
coverage only. There is no candidate evidence ingestion or verification UI in
this phase. Before using real candidate data, provide the encryption, access
controls and explicit fact-verification workflow required by the product
specification. The job-observation producer is awaiting the scope decision on
issue #19; the evaluator alone does not automatically fetch job postings.
