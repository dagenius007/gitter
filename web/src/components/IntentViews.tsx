import React from "react";

export type IntentPayload = Record<string, unknown> | undefined;
export type IntentMeta = { type?: string; payload?: IntentPayload } | undefined;

export type PR = {
  number: number;
  title: string;
  author: string;
  status: string;
  url: string;
  repository: string;
};

export type Comment = {
  author: string;
  body: string;
  timestamp: string;
  type: string;
  path?: string;
  line?: number;
};

export function renderIntentContent(intent: IntentMeta) {
  const t = intent?.type;
  const p = (intent?.payload || {}) as Record<string, unknown>;
  if (!t) return null;
  if (t === "show_prs") {
    const kind = (p["kind"] as string) || "mine";
    const prs = Array.isArray(p["prs"]) ? (p["prs"] as PR[]) : [];
    return <PRList kind={kind} prs={prs} />;
  }
  if (t === "show_comments") {
    const repo = (p["repo"] as string) || "";
    const prNumber = (p["prNumber"] as number) || 0;
    const comments = Array.isArray(p["comments"])
      ? (p["comments"] as Comment[])
      : [];
    return <PRComments repo={repo} prNumber={prNumber} comments={comments} />;
  }
  return null;
}

function PRList({ kind, prs }: { kind: string; prs: PR[] }) {
  return (
    <div className="intent intent-prlist">
      <div className="intent-title">
        {kind === "review" ? "PRs To Review" : "My PRs"}
      </div>
      {prs.length === 0 ? (
        <div className="empty">No PRs found.</div>
      ) : (
        <ul className="pr-list">
          {prs.map((p) => (
            <li key={`${p.repository}#${p.number}`}>
              <div className="pr-head">
                <span className="pr-num">#{p.number}</span>
                <span className="pr-title">{p.title}</span>
              </div>
              <div className="pr-sub">
                <span className="pr-repo">{p.repository}</span>
                <span className="dot">·</span>
                <span className="pr-author">{p.author}</span>
                {p.status ? (
                  <>
                    <span className="dot">·</span>
                    <span className="pr-status">{p.status}</span>
                  </>
                ) : null}
                {p.url ? (
                  <>
                    <span className="dot">·</span>
                    <a
                      className="pr-link"
                      href={p.url}
                      target="_blank"
                      rel="noreferrer"
                    >
                      View
                    </a>
                  </>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function PRComments({
  repo,
  prNumber,
  comments,
}: {
  repo: string;
  prNumber: number;
  comments: Comment[];
}) {
  return (
    <div className="intent intent-comments">
      <div className="intent-title">
        Comments on {repo}#{prNumber}
      </div>
      {comments.length === 0 ? (
        <div className="empty">No comments.</div>
      ) : (
        <ul className="comment-list">
          {comments.map((c, idx) => (
            <li key={idx} className="comment-item">
              <div className="c-head">
                <span className="c-author">{c.author}</span>
                <span className="dot">·</span>
                <span className="c-time">{c.timestamp}</span>
                {c.type ? (
                  <>
                    <span className="dot">·</span>
                    <span className="c-type">{c.type}</span>
                  </>
                ) : null}
                {c.path ? (
                  <>
                    <span className="dot">·</span>
                    <span className="c-loc">
                      {c.path}
                      {c.line ? `:${c.line}` : ""}
                    </span>
                  </>
                ) : null}
              </div>
              <div className="c-body">{c.body}</div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// Removed non-displayed intents (clarify, merged) per current scope
