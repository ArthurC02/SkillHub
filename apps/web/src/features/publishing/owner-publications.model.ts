import type { OwnerCatalogExposureStateEnum } from "@skillhub/api-client-ts";
import type { OwnerPublicationSummary } from "./publishing.service";
import { isDate, isRecord, isUuid } from "../../core/api/wire";
import { isAcquisition, isLabelled } from "./availability.model";

const exposureStates: Record<OwnerCatalogExposureStateEnum, true> = {
  listed: true,
  awaiting_review: true,
  revoked: true,
  review_outdated: true,
  not_eligible: true,
  search_not_ready: true,
  unreleased: true,
};

function isOptionalRelease(value: unknown) {
  return (
    value === undefined ||
    (isRecord(value) &&
      isUuid(value.version_id) &&
      typeof value.version_number === "number" &&
      Number.isInteger(value.version_number) &&
      isDate(value.released_at))
  );
}

function isExposure(value: unknown) {
  return (
    isRecord(value) && typeof value.state === "string" && Object.hasOwn(exposureStates, value.state)
  );
}

function isPublication(value: unknown) {
  if (!isRecord(value)) return false;
  const availability = value.availability;
  const acquisition = value.acquisition;
  return (
    isUuid(value.skill_id) &&
    typeof value.publisher === "string" &&
    typeof value.name === "string" &&
    typeof value.address === "string" &&
    (value.status === "published" || value.status === "delisted") &&
    isDate(value.status_changed_at) &&
    isExposure(value.catalog_exposure) &&
    isOptionalRelease(value.latest_release) &&
    (availability === undefined || isLabelled(availability)) &&
    (acquisition === undefined || isAcquisition(acquisition))
  );
}

export function readOwnerPublications(value: unknown): { publications: OwnerPublicationSummary[] } {
  if (
    !isRecord(value) ||
    !Array.isArray(value.publications) ||
    !value.publications.every(isPublication)
  )
    throw new Error("無法確認小工具發佈清單的資料。");
  return value as unknown as { publications: OwnerPublicationSummary[] };
}
