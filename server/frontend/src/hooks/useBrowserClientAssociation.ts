import { useCallback, useEffect, useMemo, useState } from 'react'
import type { DeviceEndpoint } from '@/api/client'
import {
  BrowserClientAssociation,
  resolveBrowserClientEndpointID,
} from '@/lib/browserClientAssociation'

export function useBrowserClientAssociation(devices: DeviceEndpoint[]) {
  const [revision, setRevision] = useState(0)

  useEffect(
    () => BrowserClientAssociation.subscribe(() => setRevision((value) => value + 1)),
    [],
  )

  const storedID = useMemo(
    () => BrowserClientAssociation.get(),
    [revision],
  )
  const associatedID = useMemo(
    () => resolveBrowserClientEndpointID(storedID, devices.map((device) => device.id)),
    [devices, storedID],
  )
  const associated = useMemo(
    () => devices.find((device) => device.id === associatedID),
    [associatedID, devices],
  )
  const selectEndpoint = useCallback(
    (endpointID: string) => BrowserClientAssociation.set(endpointID),
    [],
  )

  return {
    associated,
    associatedID,
    selectEndpoint,
    storedID,
  }
}
