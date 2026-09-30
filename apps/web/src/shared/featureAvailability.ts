import { createContext, useContext } from "react";

export const FeatureAvailabilityContext = createContext({ creation: false });

export function useFeatureAvailability() {
  return useContext(FeatureAvailabilityContext);
}
