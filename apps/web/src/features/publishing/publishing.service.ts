import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiFetch,
  ApiError,
  isWriteResultUnknown,
  SessionNotReadyError,
} from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type { DeclarableLicense, OwnerCatalogExposureStateEnum } from "@skillhub/api-client-ts";
import type { CategorizedFindings, Labelled } from "../../core/api/types";
import { readPublicPublication } from "./public-publication.model";
import { readOwnerPublications } from "./owner-publications.model";
import { AcquisitionResultUnknown, readAcquisition } from "./acquisition.model";

export interface Publisher {
  name: string;
  created_at: string;
}

export type PublicationKind = "skill" | "bundle";

export interface PublicationRelease {
  version_id?: string;
  version_number?: number;
  bundle_version?: string;
  content_hash: string;
  released_at: string;
  rights_attested: boolean;
  declared_license?: DeclarableLicense;
  findings: CategorizedFindings;
}

export interface Publication {
  kind: PublicationKind;
  publisher: string;
  name: string;
  address: string;
  status: "published" | "delisted";
  status_changed_at: string;
  releases: PublicationRelease[];
  availability?: Labelled;
  acquisition?: PublicationNote;
}

export interface OwnerPublicationSummary {
  skill_id: string;
  publisher: string;
  name: string;
  address: string;
  status: "published" | "delisted";
  status_changed_at: string;
  catalog_exposure: {
    state: OwnerCatalogExposureStateEnum;
  };
  latest_release?: {
    version_id: string;
    version_number: number;
    released_at: string;
  };
  availability?: Labelled;
  acquisition?: PublicationNote;
}

export interface BundleMemberChange {
  name: string;
  change: "added" | "removed" | "changed";
  from?: number;
  to?: number;
}

export interface PublicRelease {
  version_number?: number;
  version?: string;
  content_hash: string;
  released_at: string;
  changes?: BundleMemberChange[];
}

export interface PublicationNote {
  available: boolean;
  note: string;
}

export interface PublicCurrentRelease extends PublicRelease {
  findings: CategorizedFindings;
  license: { expression: string; source: string };
  redistribution: Labelled;
}

export interface PublicBundleMember {
  name: string;
  version_number: number;
  content_hash: string;
}

export interface PublicBundle {
  version: string;
  description: string;
  members: PublicBundleMember[];
}

export interface PublicBundleRelease extends PublicRelease {
  findings: CategorizedFindings;
}

export interface PublicPublication {
  kind: PublicationKind;
  publisher: string;
  name: string;
  address: string;
  availability: Labelled;
  delisted_at?: string;
  skill?: { name: string; summary: string };
  release?: PublicCurrentRelease;
  bundle?: PublicBundle;
  bundle_release?: PublicBundleRelease;
  releases: PublicRelease[];
  exposure: PublicationNote;
  acquisition: PublicationNote;
}

export interface BundleMember {
  skill_id: string;
  version_id: string;
  name: string;
  version_number: number;
  content_hash: string;
}

export interface BundleVersion {
  bundle: string;
  version: string;
  description: string;
  content_hash: string;
  created_at: string;
  members: BundleMember[];
}

export interface OwnerBundlePublicationSummary {
  publisher: string;
  name: string;
  address: string;
  status: "published" | "delisted";
  status_changed_at: string;
  latest_release?: {
    bundle_version: string;
    content_hash: string;
    released_at: string;
  };
  availability?: Labelled;
  acquisition?: PublicationNote;
}

export interface OwnerBundleSummary {
  bundle: string;
  latest_version: BundleVersion;
  publication?: OwnerBundlePublicationSummary;
}

export interface Acquisition {
  artifact_id: string;
  file_name: string;
  size_bytes: number;
  content_hash: string;
  expires_at: string;
  duplicate: boolean;
  content_url: string;
}

export function useOwnPublisher(enabled = true) {
  return useQuery({
    queryKey: queryKeys.publishing.publisher,
    queryFn: () => apiFetch<Publisher>("/me/publisher"),
    enabled,
  });
}

export function useOwnPublications(enabled = true) {
  return useQuery({
    queryKey: queryKeys.publishing.publications,
    queryFn: async () => readOwnerPublications(await apiFetch<unknown>("/me/publications")),
    enabled,
  });
}

export function useRegisterPublisher() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      apiFetch<Publisher>("/me/publisher", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      }),
    onSettled: (_, error) => {
      if (error instanceof SessionNotReadyError) return;
      void client.invalidateQueries({ queryKey: queryKeys.publishing.publisher });
    },
  });
}

export function useOwnPublication(skillId: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.publishing.ownPublication(skillId),
    queryFn: () => apiFetch<Publication>(`/skills/${skillId}/publication`),
    enabled: skillId.length > 0 && enabled,
  });
}

export interface PublishRequest {
  name?: string;
  versionId?: string;
  rightsAttested?: boolean;
  declaredLicense?: DeclarableLicense | "";
}

export function usePublish(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ name, versionId, rightsAttested, declaredLicense }: PublishRequest) =>
      apiFetch<Publication>(`/skills/${skillId}/publication`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          version_id: versionId,
          rights_attested: rightsAttested,
          declared_license: declaredLicense || undefined,
        }),
      }),
    onSettled: (_, error) => {
      if (error instanceof SessionNotReadyError) return;
      void client.invalidateQueries({ queryKey: queryKeys.publishing.ownPublication(skillId) });
      void client.invalidateQueries({ queryKey: queryKeys.publishing.publications });
    },
  });
}

export function useDelist(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<Publication>(`/skills/${skillId}/publication`, { method: "DELETE" }),
    onSettled: (_, error) => {
      if (error instanceof SessionNotReadyError) return;
      void client.invalidateQueries({ queryKey: queryKeys.publishing.ownPublication(skillId) });
      void client.invalidateQueries({ queryKey: queryKeys.publishing.publications });
    },
  });
}

export function usePublicPublication(publisher: string, name: string) {
  return useQuery({
    queryKey: queryKeys.publishing.publicPublication(publisher, name),
    queryFn: async () =>
      readPublicPublication(await apiFetch<unknown>(`/publications/${publisher}/${name}`)),
    enabled: publisher.length > 0 && name.length > 0,
  });
}

async function requestAcquisition(path: string) {
  try {
    return readAcquisition(await apiFetch<unknown>(path, { method: "POST" }));
  } catch (error) {
    if (isWriteResultUnknown(error)) throw new AcquisitionResultUnknown();
    throw error;
  }
}

export function useAcquirePublication(publisher: string, name: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => requestAcquisition(`/publications/${publisher}/${name}/acquisitions`),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.packaging.downloads }),
    onError: (error) => {
      if (error instanceof ApiError && (error.status === 404 || error.status === 409)) {
        void client.resetQueries({
          queryKey: queryKeys.publishing.publicPublication(publisher, name),
          exact: true,
        });
      }
    },
  });
}

export function useOwnBundles() {
  return useQuery({
    queryKey: queryKeys.publishing.bundles,
    queryFn: () => apiFetch<{ bundles: BundleVersion[] }>("/me/bundles"),
  });
}

export function useOwnBundleOverview(enabled = true) {
  return useQuery({
    queryKey: queryKeys.publishing.bundleOverview,
    queryFn: () => apiFetch<{ bundles: OwnerBundleSummary[] }>("/me/bundles/overview"),
    enabled,
  });
}

export interface CreateBundleRequest {
  name: string;
  version: string;
  description: string;
  memberVersionIds: string[];
}

export function useCreateBundleVersion() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ name, version, description, memberVersionIds }: CreateBundleRequest) =>
      apiFetch<BundleVersion>("/me/bundles", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          version,
          description,
          member_version_ids: memberVersionIds,
        }),
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: queryKeys.publishing.bundles });
      void client.invalidateQueries({ queryKey: queryKeys.publishing.bundleOverview });
    },
  });
}

export function useExportBundle(bundle: string, version: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      requestAcquisition(`/me/bundles/${bundle}/export?version=${encodeURIComponent(version)}`),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.packaging.downloads }),
  });
}

export interface PublishBundleRequest {
  name?: string;
  version?: string;
  rightsAttested?: boolean;
}

export function usePublishBundle(bundle: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ name, version, rightsAttested }: PublishBundleRequest) =>
      apiFetch<Publication>(`/me/bundles/${bundle}/publication`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, version, rights_attested: rightsAttested }),
      }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: queryKeys.publishing.bundleOverview });
    },
  });
}

export function useDelistBundle(bundle: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<Publication>(`/me/bundles/${bundle}/publication`, { method: "DELETE" }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: queryKeys.publishing.bundleOverview });
    },
  });
}
