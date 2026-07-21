# Project Rules — Tsunami

## Git: commit & push

The main branch is **`master`**. Follow these rules for every change:

1. **Never push to `master`.** `master` is protected — do not commit to it directly and
   do not `git push` to it.
2. **Always work on a new branch** whose name reflects the change
   (e.g. `worker-docs`, `fix-metrics-nan`, `add-grpc-start-response`). Create it before
   committing.
3. **Write meaningful commit messages** that explain *what* changed and *why*.
   **Do NOT add any Claude/AI credit** — no `Co-Authored-By: Claude ...`, no
   "Generated with Claude Code" footer, nothing attributing the work to an AI.
4. **Author commits as the `pps` user.** Use:
   ```bash
   git -c user.name="pps" -c user.email="pps.fullstack@gmail.com" commit -m "..."
   ```
   (or ensure `user.name=pps` is set for the commit).

### Typical flow
```bash
git checkout -b <branch-reflecting-the-work>
git add <files>
git -c user.name="pps" -c user.email="pps.fullstack@gmail.com" commit -m "meaningful message"
git push -u origin <branch>          # push the branch, never master
# open a PR into master
```
