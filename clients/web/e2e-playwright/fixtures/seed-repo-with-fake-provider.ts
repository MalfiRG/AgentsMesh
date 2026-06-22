import * as http from "node:http";
import { DbFixture } from "./db.fixture";
import { TEST_ORG_SLUG } from "../helpers/env";
import { uniqueSuffix } from "../helpers/test-data";

export interface SeedRepoOptions {
  branches?: string[];
  status?: number;
}

export interface SeededRepo {
  repoId: number;
  slug: string;
  providerUrl: string;
  cleanup: () => void;
}

function startFakeGitServer(opts: SeedRepoOptions): Promise<{
  url: string;
  close: () => Promise<void>;
}> {
  const branches = opts.branches ?? [];
  const statusCode = opts.status ?? 200;

  const server = http.createServer((req, res) => {
    if (statusCode !== 200) {
      res.writeHead(statusCode, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ message: "error" }));
      return;
    }
    if (req.url?.endsWith("/branches")) {
      const payload = branches.map((name) => ({
        name,
        commit: { sha: "000000" },
        protected: false,
      }));
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(payload));
      return;
    }
    if (req.url && /\/repos\//.test(req.url) && !req.url.endsWith("/branches")) {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(
        JSON.stringify({
          id: 1,
          name: "fake-repo",
          full_name: "e2e-org/fake-repo",
          default_branch: "main",
        }),
      );
      return;
    }
    res.writeHead(404);
    res.end("not found");
  });

  return new Promise((resolve, reject) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      if (!addr || typeof addr === "string") {
        reject(new Error("unexpected server address"));
        return;
      }
      const url = `http://127.0.0.1:${addr.port}`;
      resolve({
        url,
        close: () =>
          new Promise<void>((res, rej) =>
            server.close((err) => (err ? rej(err) : res())),
          ),
      });
    });
    server.on("error", reject);
  });
}

export async function seedRepoWithFakeProvider(
  db: DbFixture,
  opts: SeedRepoOptions = {},
): Promise<SeededRepo> {
  const { url: providerUrl, close } = await startFakeGitServer(opts);

  const suffix = uniqueSuffix();
  const externalId = `e2e-org/fake-repo-${suffix}`;
  const slug = `fake-repo-${suffix}`;
  const repoName = `fake-repo-${suffix}`;

  const orgIdRow = db.queryValue(
    `SELECT id FROM organizations WHERE slug = '${TEST_ORG_SLUG}'`,
  );
  if (!orgIdRow) throw new Error(`org ${TEST_ORG_SLUG} not found in DB`);
  const orgId = orgIdRow;

  const userIdRow = db.queryValue(
    `SELECT id FROM users WHERE email = 'dev@agentsmesh.local'`,
  );
  if (!userIdRow) throw new Error("dev user not found in DB");
  const userId = userIdRow;

  db.setup(`
    INSERT INTO repositories
      (organization_id, provider_type, provider_base_url, external_id, name, slug, default_branch, visibility, is_active)
    VALUES
      (${orgId}, 'github', '${providerUrl}', '${externalId}', '${repoName}', '${slug}', 'main', 'organization', true)
  `);

  const repoIdRow = db.queryValue(
    `SELECT id FROM repositories WHERE slug = '${slug}' AND organization_id = ${orgId}`,
  );
  if (!repoIdRow) throw new Error("seeded repo not found after insert");
  const repoId = Number(repoIdRow);

  db.setup(`
    INSERT INTO user_repository_providers
      (user_id, provider_type, name, base_url, bot_token_encrypted, is_default, is_active)
    VALUES
      (${userId}, 'github', 'e2e-fake-${suffix}', '${providerUrl}', 'e2e-token', false, true)
  `);

  const providerIdRow = db.queryValue(
    `SELECT id FROM user_repository_providers WHERE name = 'e2e-fake-${suffix}' AND user_id = ${userId}`,
  );

  function cleanup(): void {
    if (providerIdRow) {
      db.cleanup(
        `DELETE FROM user_repository_providers WHERE id = ${providerIdRow}`,
      );
    }
    db.cleanup(`DELETE FROM repositories WHERE id = ${repoId}`);
    void close();
  }

  return { repoId, slug, providerUrl, cleanup };
}
