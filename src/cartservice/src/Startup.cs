using System;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Diagnostics.HealthChecks;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Diagnostics.HealthChecks;
using Microsoft.Extensions.Hosting;
using cartservice.cartstore;
using OpenTelemetry.Trace;

namespace cartservice
{
    public class Startup
    {
        public Startup(IConfiguration configuration)
        {
            Configuration = configuration;
        }

        public IConfiguration Configuration { get; }
        
        public void ConfigureServices(IServiceCollection services)
        {
            string mongoUri  = Configuration["MONGO_URI"];
            string mongoDatabase = Configuration["MONGO_DATABASE"];
            string redisAddr = Configuration["REDIS_ADDR"];

            if (!string.IsNullOrEmpty(mongoUri))
            {
                Console.WriteLine("Creating MongoDB cart store");
                services.AddSingleton<ICartStore>(new MongoCartStore(mongoUri, mongoDatabase));
            }
            else if (!string.IsNullOrEmpty(redisAddr))
            {
                services.AddStackExchangeRedisCache(options =>
                {
                    options.Configuration = redisAddr;
                });
                services.AddSingleton<ICartStore, RedisCartStore>();
            }
            else
            {
                Console.WriteLine("No DB configured. Using in-memory store.");
                services.AddDistributedMemoryCache();
                services.AddSingleton<ICartStore, RedisCartStore>();
            }

            services.AddControllers();
            services.AddHealthChecks();

            if (string.IsNullOrEmpty(Environment.GetEnvironmentVariable("DISABLE_TRACING")))
            {
                services.AddOpenTelemetry()
                    .WithTracing(builder => builder
                        .AddAspNetCoreInstrumentation()
                        .AddOtlpExporter());
            }
        }

        public void Configure(IApplicationBuilder app, IWebHostEnvironment env)
        {
            if (env.IsDevelopment())
            {
                app.UseDeveloperExceptionPage();
            }

            app.UseHealthChecks("/_healthz");
            app.UseRouting();

            app.UseEndpoints(endpoints =>
            {
                endpoints.MapControllers();
            });
        }
    }
}
