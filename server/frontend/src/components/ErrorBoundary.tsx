import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

type Props = { children: ReactNode }
type State = { error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('ErrorBoundary', error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex min-h-[40vh] flex-col items-center justify-center gap-4 p-8 text-center">
          <h1 className="text-lg font-semibold text-mga-text">Something went wrong</h1>
          <p className="max-w-md text-sm text-mga-muted">{this.state.error.message}</p>
          <Button
            type="button"
            variant="outline"
            onClick={() => this.setState({ error: null })}
          >
            Try again
          </Button>
        </div>
      )
    }
    return this.props.children
  }
}
