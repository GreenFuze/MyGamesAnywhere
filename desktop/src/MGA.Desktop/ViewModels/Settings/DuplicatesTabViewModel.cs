using System.Collections.ObjectModel;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels.Settings;

/// <summary>Display model for a single duplicate-group row.</summary>
public sealed class DuplicateGroupRowModel
{
    public string RepresentativeTitle { get; init; } = string.Empty;
    public int    SourceCount         { get; init; }
    public string SizeText            { get; init; } = string.Empty;
}

/// <summary>
/// Duplicates tab — lists groups of duplicate games with their source counts and total sizes.
/// Data is fetched from GET /api/duplicates/games on construction and on manual reload.
/// </summary>
public sealed partial class DuplicatesTabViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly ToastService            _toast;

    // ---------------------------------------------------------------------------
    // Observable state
    // ---------------------------------------------------------------------------

    [ObservableProperty]
    private bool _isLoading;

    [ObservableProperty]
    private int _groupCount;

    [ObservableProperty]
    private ObservableCollection<DuplicateGroupRowModel> _groups = [];

    // ---------------------------------------------------------------------------
    // Constructor
    // ---------------------------------------------------------------------------

    public DuplicatesTabViewModel(ServerConnectionService server, ToastService toast)
    {
        _server = server;
        _toast  = toast;

        _ = LoadAsync();
    }

    // ---------------------------------------------------------------------------
    // Commands
    // ---------------------------------------------------------------------------

    [RelayCommand]
    private Task ReloadAsync() => LoadAsync();

    // ---------------------------------------------------------------------------
    // Private — data loading
    // ---------------------------------------------------------------------------

    private async Task LoadAsync()
    {
        if (_server.Api is null) return;

        IsLoading = true;

        try
        {
            var response = await _server.Api.GetDuplicatesAsync().ConfigureAwait(true);

            GroupCount = response.Groups.Count;
            Groups = new ObservableCollection<DuplicateGroupRowModel>(
                response.Groups.Select(g => new DuplicateGroupRowModel
                {
                    RepresentativeTitle = g.RepresentativeTitle,
                    SourceCount         = g.Sources.Count,
                    SizeText            = ByteFormatter.Format(g.Sources.Sum(s => s.TotalSize)),
                }));
        }
        catch (Exception ex)
        {
            _toast.Error("Failed to load duplicates", ex.Message);
        }
        finally
        {
            IsLoading = false;
        }
    }

}
