namespace MGA.Api;

/// <summary>
/// Thrown by MgaApiService when the server returns a non-2xx response.
/// Always contains the HTTP status code and a structured error code where available.
/// Callers must handle this explicitly — MgaApiService never swallows errors silently.
/// </summary>
public sealed class MgaApiException : Exception
{
    public int StatusCode { get; }

    /// <summary>Machine-readable error code from the server response body, if present.</summary>
    public string? ErrorCode { get; }

    public MgaApiException(int statusCode, string message, string? errorCode = null)
        : base(message)
    {
        StatusCode = statusCode;
        ErrorCode = errorCode;
    }

    public MgaApiException(int statusCode, string message, Exception inner, string? errorCode = null)
        : base(message, inner)
    {
        StatusCode = statusCode;
        ErrorCode = errorCode;
    }

    public override string ToString() =>
        $"MgaApiException({StatusCode}, code={ErrorCode ?? "none"}): {Message}";
}
