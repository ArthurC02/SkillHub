import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../core/api/client";
import { queryKeys } from "../../core/api/queryKeys";
import type { CategorizedFindings, Labelled } from "../../core/api/types";

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

export function useRegisterPublisher() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (name: string) =>
      apiFetch<Publisher>("/me/publisher", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.publishing.publisher }),
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
}

export function usePublish(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ name, versionId, rightsAttested }: PublishRequest) =>
      apiFetch<Publication>(`/skills/${skillId}/publication`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, version_id: versionId, rights_attested: rightsAttested }),
      }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: queryKeys.publishing.ownPublication(skillId) }),
  });
}

export function useDelist(skillId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<Publication>(`/skills/${skillId}/publication`, { method: "DELETE" }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: queryKeys.publishing.ownPublication(skillId) }),
  });
}

export function usePublicPublication(publisher: string, name: string) {
  return useQuery({
    queryKey: queryKeys.publishing.publicPublication(publisher, name),
    queryFn: () => apiFetch<PublicPublication>(`/publications/${publisher}/${name}`),
    enabled: publisher.length > 0 && name.length > 0,
  });
}

export function useAcquirePublication(publisher: string, name: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<Acquisition>(`/publications/${publisher}/${name}/acquisitions`, { method: "POST" }),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.packaging.downloads }),
  });
}

export function useOwnBundles() {
  return useQuery({
    queryKey: queryKeys.publishing.bundles,
    queryFn: () => apiFetch<{ bundles: BundleVersion[] }>("/me/bundles"),
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
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.publishing.bundles }),
  });
}

export function useExportBundle(bundle: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () => apiFetch<Acquisition>(`/me/bundles/${bundle}/export`, { method: "POST" }),
    onSuccess: () => client.invalidateQueries({ queryKey: queryKeys.packaging.downloads }),
  });
}

export function useOwnBundlePublication(bundle: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.publishing.ownBundlePublication(bundle),
    queryFn: () => apiFetch<Publication>(`/me/bundles/${bundle}/publication`),
    enabled: bundle.length > 0 && enabled,
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
    onSuccess: () =>
      client.invalidateQueries({ queryKey: queryKeys.publishing.ownBundlePublication(bundle) }),
  });
}

export function useDelistBundle(bundle: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiFetch<Publication>(`/me/bundles/${bundle}/publication`, { method: "DELETE" }),
    onSuccess: () =>
      client.invalidateQueries({ queryKey: queryKeys.publishing.ownBundlePublication(bundle) }),
  });
}
