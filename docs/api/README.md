# Git LFS API

The Git LFS client uses an HTTPS server to coordinate fetching and storing
large binary objects separately from a Git server. The basic process the client
goes through looks like this:

1. [Discover the LFS Server to use](./server-discovery.md).
2. [Apply Authentication](./authentication.md).
3. Make the request. See the Batch and File Locking API sections.

## Batch API

The Batch API is used to request the ability to transfer LFS objects with the
LFS server.

API Specification:
  * [Batch API](./batch.md)

Current transfer adapters include:
  * [Basic](./basic-transfers.md)
  * [Static](./static-transfers.md)

Experimental transfer adapters include:
  * Tus.io (upload only)
  * [Custom](../custom-transfers.md)

## File Locking API

The File Locking API is used to create, list, and delete locks, as well as
verify that locks are respected in Git pushes.

API Specification:
  * [File Locking API](./locking.md)
