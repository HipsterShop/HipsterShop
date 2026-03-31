
using System.Threading.Tasks;
using Microsoft.AspNetCore.Mvc;
using cartservice.cartstore;
using cartservice.models;

namespace cartservice.controllers
{
    [ApiController]
    public class CartController : ControllerBase
    {
        private readonly ICartStore _cartStore;

        public CartController(ICartStore cartStore)
        {
            _cartStore = cartStore;
        }

        [HttpPost("/cart/add")]
        public async Task<IActionResult> AddItem([FromBody] AddItemRequest request)
        {
            await _cartStore.AddItemAsync(request.UserId, request.Item.ProductId, request.Item.Quantity);
            return Ok(new {});
        }

        [HttpPost("/cart/get")]
        public async Task<IActionResult> GetCart([FromBody] GetCartRequest request)
        {
            var cart = await _cartStore.GetCartAsync(request.UserId);
            return Ok(cart);
        }

        [HttpPost("/cart/empty")]
        public async Task<IActionResult> EmptyCart([FromBody] EmptyCartRequest request)
        {
            await _cartStore.EmptyCartAsync(request.UserId);
            return Ok(new {});
        }

    }
}
