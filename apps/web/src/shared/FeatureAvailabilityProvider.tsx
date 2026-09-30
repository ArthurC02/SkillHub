import type { ReactNode } from "react";
import { FeatureAvailabilityContext } from "./featureAvailability";

export function FeatureAvailabilityProvider({
  creation,
  children,
}: {
  creation: boolean;
  children: ReactNode;
}) {
  return (
    <FeatureAvailabilityContext.Provider value={{ creation }}>
      {children}
    </FeatureAvailabilityContext.Provider>
  );
}
