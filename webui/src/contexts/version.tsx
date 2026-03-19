import { createContext, ReactNode, useEffect, useState } from 'react'

import { BASE_PATH } from 'libs/utils'

type VersionContextProps = {
  version: string
  dashboardName: string
}

export const VersionContext = createContext<VersionContextProps>({
  version: '',
  dashboardName: '',
})

type VersionProviderProps = {
  children: ReactNode
}

export const VersionProvider = ({ children }: VersionProviderProps) => {
  const [version, setVersion] = useState('')
  const [dashboardName, setDashboardName] = useState('')

  useEffect(() => {
    const fetchVersion = async () => {
      try {
        const response = await fetch(`${BASE_PATH}/version`)
        if (!response.ok) {
          throw new Error(`Network error: ${response.status}`)
        }
        const data: API.Version = await response.json()
        setVersion(data.Version)
        setDashboardName(data.dashboardName || '')
      } catch (err) {
        console.error(err)
      }
    }

    fetchVersion()
  }, [])

  return <VersionContext.Provider value={{ version, dashboardName }}>{children}</VersionContext.Provider>
}
