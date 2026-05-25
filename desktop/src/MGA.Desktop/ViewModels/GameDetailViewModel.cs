using CommunityToolkit.Mvvm.ComponentModel;
using MGA.Desktop.Services;

namespace MGA.Desktop.ViewModels;

/// <summary>
/// Game detail page — full-bleed hero banner, metadata panel, media carousel.
/// Scaffold stub: full implementation added in the next iteration.
/// </summary>
public sealed partial class GameDetailViewModel : ViewModelBase
{
    private readonly ServerConnectionService _server;
    private readonly NavigationService       _nav;
    private readonly ToastService            _toast;

    [ObservableProperty]
    private string _gameId = string.Empty;

    public GameDetailViewModel(
        string                  gameId,
        ServerConnectionService server,
        NavigationService       nav,
        ToastService            toast)
    {
        GameId  = gameId;
        _server = server;
        _nav    = nav;
        _toast  = toast;
    }
}
