using System.Net.Http.Json;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace MGA.Api;

/// <summary>
/// Facade over the generated MGA REST API client.
///
/// All methods throw MgaApiException on non-2xx responses — never return null
/// or swallow errors silently (fail-fast policy).
///
/// The HttpClient lifetime is owned by ServerConnectionService (RAII).
/// This class is a thin stateless wrapper — safe to re-create when switching servers.
///
/// TODO: after running scripts/generate-api-client.ps1, replace the stub methods
///       below with calls to the generated MgaApiClient.
/// </summary>
public sealed class MgaApiService
{
    private readonly HttpClient _http;

    internal static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        NumberHandling = JsonNumberHandling.AllowReadingFromString,
    };

    public MgaApiService(HttpClient http)
    {
        _http = http;
    }

    // ---------------------------------------------------------------------------
    // Health
    // ---------------------------------------------------------------------------

    /// <summary>Returns true when the server responds with HTTP 200 on /health.</summary>
    public async Task<bool> PingAsync(CancellationToken ct = default)
    {
        try
        {
            var resp = await _http.GetAsync("/health", ct).ConfigureAwait(false);
            return resp.IsSuccessStatusCode;
        }
        catch
        {
            return false;
        }
    }

    // ---------------------------------------------------------------------------
    // Internal helpers
    // ---------------------------------------------------------------------------

    private async Task<T> GetAsync<T>(string path, CancellationToken ct = default)
    {
        var resp = await _http.GetAsync(path, ct).ConfigureAwait(false);
        await EnsureSuccess(resp, ct).ConfigureAwait(false);
        var result = await resp.Content.ReadFromJsonAsync<T>(JsonOptions, ct).ConfigureAwait(false);
        return result ?? throw new MgaApiException(200, $"Server returned null for {path}");
    }

    private async Task EnsureSuccess(HttpResponseMessage resp, CancellationToken ct)
    {
        if (resp.IsSuccessStatusCode)
            return;

        // Try to extract a structured error body.
        string? errorCode = null;
        string message;

        try
        {
            var body = await resp.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
            using var doc = JsonDocument.Parse(body);

            errorCode = doc.RootElement.TryGetProperty("code", out var codeEl)
                ? codeEl.GetString()
                : null;

            message = doc.RootElement.TryGetProperty("message", out var msgEl)
                ? msgEl.GetString() ?? resp.ReasonPhrase ?? "unknown error"
                : body;
        }
        catch
        {
            message = resp.ReasonPhrase ?? "unknown error";
        }

        throw new MgaApiException((int)resp.StatusCode, message, errorCode);
    }

    // ---------------------------------------------------------------------------
    // TODO: replace stubs below with generated client calls after running
    //       desktop/scripts/generate-api-client.ps1
    // ---------------------------------------------------------------------------
    // Example generated shape:
    //   public Task<GameListResponse> ListGamesAsync(int page = 0, int pageSize = 100, CancellationToken ct = default)
    //       => GetAsync<GameListResponse>($"/api/games?page={page}&page_size={pageSize}", ct);
}
