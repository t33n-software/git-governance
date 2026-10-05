# Branch hygiene: actor-invoked, evidence-based deletion

## Purpose

This convention defines how local official working branches are deleted on
a developer machine. Hygiene runs exclusively through the actor-invoked,
evidence-based governed capability of this binary: never automatically,
never in the background, and never as a raw Git deletion. It closes the gap
between the physical branch surface of a developer machine and the governed
branch lifecycle — completed and aborted working branches become
hygiene-eligible under their own evidence, and every deletion is its own
confirmed, audit-recorded dispatch.

The worktree side of the cleanup pairing — the isolation model and the
governed worktree lifecycle — is documented in
[Worktree isolation convention](../worktrees/worktree-isolation.md) and in
the [Task worktree lifecycle](../../usage/workflows/worktrees.md) usage
surface; the allocation inventory that carries the evidence surfaces is
documented in [Ticket allocation](../tickets/ticket-allocation.md).

## The law

1. **Nothing deletes official working branches automatically.** Automatic
   deletion, IDE- or scheduler-triggered cleanup, deletion coupled to
   worktree removal, and raw Git branch deletion are forbidden forms; they
   are the standing unattended mutation channel without per-event proof.
   Every deletion event is its own confirmed, audit-recorded dispatch by
   the actor: nothing runs in the background, ever.
2. **Hygiene eligibility carries its own evidence conjunction.** A local
   official working branch is hygiene-eligible only in exactly one of two
   classes, each requiring all of its evidence at the deletion event:

| Class | Required evidence conjunction |
|---|---|
| worked-to-completion | the newest pull-request record of the branch is merged or closed; the lane's completion obligations are fulfilled; the branch holds no commits ahead of its lane base; the checkout is clean; no active operation marker holds it |
| never-published (aborted or falsely created) | no publication record exists; the branch holds no commits ahead of its lane base; the checkout is clean; no active operation marker holds it |

3. **The guards refuse fail-closed and never delete on refusal.** A
   governed hygiene dispatch refuses deletion when any of the following
   holds: the branch's newest pull request is open (the review-feedback
   continuation context); the branch holds unmerged commits ahead of its
   lane base (the loss risk is reported by name, never resolved by
   removal); the checkout is unclean or carries an active operation marker;
   or the branch belongs to a protected line (`release/*`, `support/*`,
   `hotfix/*`) whose own propagation, delivery, and reconciliation
   obligation chains are not proven complete.

## Responsibilities

- This binary is the programmatic fail-closed enforcement plane of the
  guards.
- The allocation inventory remains pure read truth: it carries the evidence
  surfaces (local and remote branch refs, pull-request records with their
  lifecycle state, commit envelopes, the task-worktree registry, hotfix
  records) and never becomes a deletion actor.
- The actor dispatches every deletion event explicitly through the governed
  capability.

## Target endpoint family

The governed branch-hygiene capability materializes as an endpoint family:
a point operation that deletes exactly one addressed working branch under
the guards, and an evidence-based discovery and batch pass — a sibling of
the worktree prune form — that derives stale-eligible candidates from the
inventory evidence, plans them without mutation, and removes each entry
under the same point guards (composition, not duplication). The concrete
endpoint form is owned by this binary's help and is derived help-first at
every invocation, never guessed.

## Do / Don't

| Do | Don't |
|---|---|
| Dispatch the governed deletion per event, confirmed and audit-recorded | Build a cron job, IDE plugin, or cleanup script that deletes branches automatically |
| Require the full evidence conjunction before deletion | Decide completion from remote state alone or from local cleanliness alone |
| Report ahead-of-base branches by name instead of removing them | Delete a branch that holds unmerged commits |
| Keep the allocation inventory a pure read surface | Turn the inventory into a deletion actor |
| Reference this convention by its own surface | Restate the guards in scripts or automation |

## Verification

A branch-hygiene practice complies with this convention when: no mechanism
anywhere in the environment deletes official working branches without an
actor-invoked governed dispatch; every deletion on record maps one-to-one
to a confirmed dispatch with its audit record; hygiene decisions cited the
full evidence conjunction of their class; ahead-of-base, open-PR, unclean,
and protected-line refusals were honored fail-closed; and the allocation
inventory was consumed read-only. A deletion without its evidence and its
audit record is a defect against this convention regardless of how finished
the branch looked.