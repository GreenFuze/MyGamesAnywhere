import { useCallback, useEffect, useMemo, useState } from 'react'
import type { DeviceEndpoint } from '@/api/client'
import { ClientEndpointAssociation, resolveAssociatedEndpointID } from '@/lib/clientEndpointAssociation'

export function useClientEndpointAssociation(profileID: string, devices: DeviceEndpoint[]) {
  const [revision, setRevision] = useState(0)

  useEffect(
    () => ClientEndpointAssociation.subscribe(profileID, () => setRevision((value) => value + 1)),
    [profileID],
  )

  const associatedID = useMemo(
    () => resolveAssociatedEndpointID(
      ClientEndpointAssociation.get(profileID),
      devices.map((device) => device.id),
    ),
    [devices, profileID, revision],
  )
  const associated = useMemo(
    () => devices.find((device) => device.id === associatedID),
    [associatedID, devices],
  )
  const selectEndpoint = useCallback(
    (endpointID: string) => ClientEndpointAssociation.set(profileID, endpointID),
    [profileID],
  )

  return {
    associated,
    associatedID,
    requiresSelection: devices.length > 1 && !associated,
    selectEndpoint,
  }
}
