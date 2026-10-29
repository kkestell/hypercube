import type { Session } from "./sessions";

export type Workspace = {
  id: string;
  name: string;
  sessions: Session[];
};

// Session ids are unique across all workspaces, since terminals are keyed by them.
export const stubWorkspaces: Workspace[] = [
  {
    id: "hc",
    name: "hc",
    sessions: [
      {
        id: "auth-refactor",
        name: "Auth refactor",
        agents: [
          { id: "planner", name: "Planner" },
          { id: "implementer", name: "Implementer", position: { relativeTo: "planner", direction: "within" } },
          { id: "reviewer", name: "Reviewer", position: { relativeTo: "planner", direction: "within" } },
        ],
      },
      {
        id: "flaky-tests",
        name: "Flaky tests",
        agents: [
          { id: "investigator", name: "Investigator" },
          { id: "fixer", name: "Fixer", position: { relativeTo: "investigator", direction: "right" } },
          { id: "test-runner", name: "Test runner", position: { relativeTo: "fixer", direction: "below" } },
          { id: "ci-watcher", name: "CI watcher", position: { relativeTo: "test-runner", direction: "within" } },
        ],
      },
      {
        id: "docs-site",
        name: "Docs site",
        agents: [
          { id: "writer", name: "Writer" },
          { id: "editor", name: "Editor", position: { relativeTo: "writer", direction: "right" } },
          { id: "designer", name: "Designer", position: { relativeTo: "writer", direction: "below" } },
          { id: "builder", name: "Builder", position: { relativeTo: "editor", direction: "below" } },
        ],
      },
    ],
  },
  {
    id: "billing-service",
    name: "billing-service",
    sessions: [
      {
        id: "invoice-export",
        name: "Invoice export",
        agents: [
          { id: "implementer", name: "Implementer" },
          { id: "reviewer", name: "Reviewer", position: { relativeTo: "implementer", direction: "right" } },
        ],
      },
      {
        id: "stripe-webhooks",
        name: "Stripe webhooks",
        agents: [
          { id: "investigator", name: "Investigator" },
          { id: "fixer", name: "Fixer", position: { relativeTo: "investigator", direction: "within" } },
        ],
      },
    ],
  },
  {
    id: "mobile-app",
    name: "mobile-app",
    sessions: [
      {
        id: "onboarding-flow",
        name: "Onboarding flow",
        agents: [
          { id: "designer", name: "Designer" },
          { id: "implementer", name: "Implementer", position: { relativeTo: "designer", direction: "within" } },
          { id: "tester", name: "Tester", position: { relativeTo: "designer", direction: "below" } },
        ],
      },
    ],
  },
  {
    id: "infra",
    name: "infra",
    sessions: [
      {
        id: "terraform-upgrade",
        name: "Terraform upgrade",
        agents: [{ id: "operator", name: "Operator" }],
      },
    ],
  },
  {
    id: "data-pipeline",
    name: "data-pipeline",
    sessions: [
      {
        id: "backfill-job",
        name: "Backfill job",
        agents: [
          { id: "runner", name: "Runner" },
          { id: "monitor", name: "Monitor", position: { relativeTo: "runner", direction: "below" } },
        ],
      },
      {
        id: "schema-migration",
        name: "Schema migration",
        agents: [
          { id: "planner", name: "Planner" },
          { id: "migrator", name: "Migrator", position: { relativeTo: "planner", direction: "right" } },
        ],
      },
    ],
  },
];
