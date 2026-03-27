
using System.Threading.Tasks;
using cartservice.models;

namespace cartservice.cartstore
{
    public interface ICartStore
    {
        Task AddItemAsync(string userId, string productId, int quantity);
        Task EmptyCartAsync(string userId);
        Task<Cart> GetCartAsync(string userId);
        bool Ping();
    }
}
