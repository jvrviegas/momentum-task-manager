# Project Instructions

Follow [`CONTRIBUTING.md`](./CONTRIBUTING.md) for project structure, validation, and safety requirements.

## Commits

- When asked to commit changes, create a separate commit for each changed file.
- Stage only the intended file and verify the staged diff before each commit.
- Never include unrelated or pre-existing working-tree changes.
- Use concise Conventional Commit messages that describe the change.

## Remote Git operations

`origin` (`jvrviegas/momentum-task-manager`) is writable only by the `jvrviegas` GitHub account, while the machine's default `gh` account is `joao-viegas-procimo`. Git authenticates through `gh`, so:

- Before any Git or `gh` operation that talks to `origin` (push, pull, fetch, PRs), switch the active account to `jvrviegas`.
- Switch back to `joao-viegas-procimo` afterwards, even when the operation fails.
- Confirm with `gh auth status` that `joao-viegas-procimo` is active again.

```sh
gh auth switch --hostname github.com --user jvrviegas
git push origin main  # the remote operation(s)
gh auth switch --hostname github.com --user joao-viegas-procimo
```

Do not stop on the remote command's failure before switching back, and do not name a shell variable `status` (it is read-only in zsh).
