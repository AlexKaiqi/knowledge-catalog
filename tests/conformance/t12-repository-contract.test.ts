import { makeRepository } from "./helpers.ts";
import { repositoryContract } from "./repository-contract.ts";

repositoryContract("file-git", makeRepository);
